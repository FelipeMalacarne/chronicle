package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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

// driveTrain is a train run's goroutine: the safe point and the checks,
// the plugin's settings, the start, then the plugin supervised until the
// proof, the timeout, a failure or the owner's cancel, which stop it.
func (e *Executor) driveTrain(ctx context.Context, p TaskRunParams) {
	tr := *p.Task.Train
	until := RunUntil{Objective: tr.Until(), Account: p.Account}
	fail := func(err error) {
		if ctx.Err() != nil {
			e.end(RunCanceled, "cancelado pelo dono; o treino parou onde estava", "")
			return
		}
		e.end(RunFailed, err.Error()+"; o treino parou onde estava", err.Error())
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
		fail(errors.New("the character is in the wilderness: an HCIM trains there only with the owner watching"))
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
	present, active, ok := e.scriptState(ctx, tr.Plugin)
	switch {
	case !ok:
		fail(errors.New("the client did not list its plugins"))
		return
	case !present:
		fail(fmt.Errorf("%s is not installed in the client (the account's profile, the fleet's model or the Hub)", tr.Plugin))
		return
	case active:
		fail(fmt.Errorf("%s is already on: stop it first, the training run starts and stops it", simple(tr.Plugin)))
		return
	}
	e.note(RunRunning, fmt.Sprintf("em %d, %d, %d; treinando %s até o ingest provar %s", at.X, at.Y, at.Plane, tr.SkillName, until.Objective))
	if others := e.activeScripts(ctx); len(others) > 0 {
		// Plugins keep passive loops all the time, so this only warns: a
		// main activity left on would click alongside the training.
		e.note(RunRunning, "plugins ligados durante o treino: "+strings.Join(others, ", ")+" (pare a atividade principal antes)")
	}
	for _, s := range tr.Config {
		body := map[string]any{"group": s.Group, "key": s.Key, "value": s.Value}
		if _, err = e.Game.JSON(ctx, http.MethodPost, "/settings/plugin", nil, body, 10*time.Second); err != nil {
			fail(fmt.Errorf("setting %s.%s: %w", s.Group, s.Key, err))
			return
		}
		e.note(RunRunning, fmt.Sprintf("configuração: %s.%s = %s", s.Group, s.Key, clip(s.Value, 80)))
	}
	before, _ := e.results(ctx, tr.Plugin)
	if _, err = e.Game.JSON(ctx, http.MethodPost, "/scripts/start", nil, map[string]any{"className": tr.Plugin}, 30*time.Second); err != nil {
		fail(fmt.Errorf("starting %s: %w", tr.Plugin, err))
		return
	}
	e.note(RunRunning, fmt.Sprintf("iniciado: %s, até o ingest provar %s", simple(tr.Plugin), until.Objective))
	timeout := untilTimeout
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	if e.taskTimeout > 0 {
		timeout = e.taskTimeout
	}
	if !e.runUntilProven(ctx, tr.Plugin, until, timeout, before, fail) {
		return
	}
	e.end(RunDone, "treino feito: "+until.Objective+" provado pelo ingest", "")
}
