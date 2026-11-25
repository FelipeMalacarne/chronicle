package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

// train by delegation (docs/10 phase 4, section 10: "train liga o plugin,
// monitora e desliga quando o objetivo fica met"): a task_run whose task is
// a train runs a plugin of the client (one the account's profile or the
// fleet's model installed, or the client already has) with the settings the
// task gives, and stops it when the server proves the level from the ingest
// (run_until, as a wait_until run). The plugin decides how to train; never
// the plugin nor the agent says when it is done. A train never goes to
// Chronicle Link's machine, but Chronicle Link must answer: its tracking is
// what the ingest proves the level from.

// delegated is a run that hands the work to a plugin of the client: a train,
// or a job's plugin task. The agent sets the plugin's settings, starts it and
// stops it when the server proves until.
type delegated struct {
	plugin string
	config []clientcfg.Setting
	until  RunUntil
	// begin is the trail line when it starts (after the position), stoppedAs
	// what the run says when it ends without the proof ("o treino parou onde
	// estava"), alongside says what a main activity left on would do, doneAs
	// the run's last line and wilderness what an HCIM in the wilderness is
	// told.
	begin, stoppedAs, alongside, doneAs, wilderness string
}

// driveTrain is a train run's goroutine.
func (e *Executor) driveTrain(ctx context.Context, p TaskRunParams) {
	tr := *p.Task.Train
	until := RunUntil{Objective: tr.Until(), Account: p.Account}
	e.driveDelegated(ctx, p, delegated{
		plugin: tr.Plugin, config: tr.Config, until: until,
		begin:      fmt.Sprintf("treinando %s até o ingest provar %s", tr.SkillName, until.Objective),
		stoppedAs:  "o treino parou onde estava",
		alongside:  "plugins ligados durante o treino: ",
		doneAs:     "treino feito: " + until.Objective + " provado pelo ingest",
		wilderness: "an HCIM trains there only with the owner watching",
	})
}

// drivePlugin is a plugin job's run goroutine.
func (e *Executor) drivePlugin(ctx context.Context, p TaskRunParams) {
	pl := *p.Task.Plugin
	until := RunUntil{Objective: pl.Until, Account: p.Account}
	e.driveDelegated(ctx, p, delegated{
		plugin: pl.Plugin, config: pl.Config, until: until,
		begin:      fmt.Sprintf("ligando %s até o ingest provar %s", pl.Name, until.Objective),
		stoppedAs:  "o plugin parou onde estava",
		alongside:  "plugins ligados durante o plugin: ",
		doneAs:     "feito: " + until.Objective + " provado pelo ingest",
		wilderness: "an HCIM runs a plugin there only with the owner watching",
	})
}

// driveDelegated is a delegated run's goroutine: the safe point and the
// checks, the plugin's settings, the start, then the plugin supervised until
// the proof, the timeout, a failure or the owner's cancel, which stop it.
func (e *Executor) driveDelegated(ctx context.Context, p TaskRunParams, d delegated) {
	until := d.until
	fail := func(err error) {
		if ctx.Err() != nil {
			e.end(RunCanceled, "cancelado pelo dono; "+d.stoppedAs, "")
			return
		}
		e.end(RunFailed, err.Error()+"; "+d.stoppedAs, err.Error())
	}
	done := func() { e.end(RunDone, "nada a fazer: o ingest já prova "+until.Objective, "") }
	if err := e.waitSafe(ctx); err != nil {
		if errors.Is(err, errUntilMet) {
			done()
			return
		}
		fail(err)
		return
	}
	st, err := e.runState(ctx)
	if err != nil {
		fail(err)
		return
	}
	if !st.LoggedIn || st.Player.Position == nil {
		fail(errors.New("the character is not logged in"))
		return
	}
	at := *st.Player.Position
	if execspec.InWilderness(at.X, at.Y) && p.Mode == content.ModeHCIM && !p.Supervised {
		fail(errors.New("the character is in the wilderness: " + d.wilderness))
		return
	}
	if e.untilMet() {
		done()
		return
	}
	// The level is proven from Chronicle Link's tracking: without it the
	// training would never end but on the timeout.
	if v := e.bridgeVersion(ctx); v == "" {
		fail(errors.New("the Chronicle Link bridge did not answer: without its tracking the ingest cannot prove the level"))
		return
	}
	present, active, ok := e.scriptState(ctx, d.plugin)
	switch {
	case !ok:
		fail(errors.New("the client did not list its plugins"))
		return
	case !present:
		fail(fmt.Errorf("%s is not installed in the client (the account's profile, the fleet's model or the Hub)", d.plugin))
		return
	case active:
		fail(fmt.Errorf("%s is already on: stop it first, the training run starts and stops it", simple(d.plugin)))
		return
	}
	e.note(RunRunning, fmt.Sprintf("em %d, %d, %d; %s", at.X, at.Y, at.Plane, d.begin))
	if others := e.activeScripts(ctx); len(others) > 0 {
		// Plugins keep passive loops all the time, so this only warns: a
		// main activity left on would click alongside the training.
		e.note(RunRunning, d.alongside+strings.Join(others, ", ")+" (pare a atividade principal antes)")
	}
	for _, s := range d.config {
		body := map[string]any{"group": s.Group, "key": s.Key, "value": s.Value}
		if _, err = e.Game.JSON(ctx, http.MethodPost, "/settings/plugin", nil, body, 10*time.Second); err != nil {
			fail(fmt.Errorf("setting %s.%s: %w", s.Group, s.Key, err))
			return
		}
		e.note(RunRunning, fmt.Sprintf("configuração: %s.%s = %s", s.Group, s.Key, clip(s.Value, 80)))
	}
	before, _ := e.results(ctx, d.plugin)
	if _, err = e.Game.JSON(ctx, http.MethodPost, "/scripts/start", nil, map[string]any{"className": d.plugin}, 30*time.Second); err != nil {
		fail(fmt.Errorf("starting %s: %w", d.plugin, err))
		return
	}
	e.note(RunRunning, fmt.Sprintf("iniciado: %s, até o ingest provar %s", simple(d.plugin), until.Objective))
	timeout := untilTimeout
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	if e.taskTimeout > 0 {
		timeout = e.taskTimeout
	}
	if !e.runUntilProven(ctx, d.plugin, until, timeout, before, fail) {
		return
	}
	e.end(RunDone, d.doneAs, "")
}
