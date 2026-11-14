package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/microbot"
)

// quest by delegation (docs/10, E19; appendix A, path 3): a task_run whose
// task is a quest selects it in the Quest Helper the pinned Microbot client
// ships, through the Agent Server's /quest-helper endpoints, and lets the
// helper's bot (its QuestScript) do it. The run ends only when the server
// proves quest:<x>@finished from the ingest (run_until, as a train's): never
// the bot nor the agent says the quest is done. Before the start the agent
// sets the helper's "Obtain Missing Items" to No (clientcfg.PinnedSettings:
// ironman, never the Grand Exchange; its default asks in a dialog nobody
// answers). The proof, the timeout, a failure, the owner's cancel and the bot
// stopping by itself all end with /quest-helper/stop. Nothing of the Quest
// Helper's code or data is in Chronicle (E20): the client brings it.

// CmdQuestHelper reads the client's Quest Helper: whether it is on, what it
// follows, and the quests it lists, by which the server names a quest task
// before the run. It never touches the game.
const CmdQuestHelper = "quest_helper"

// QuestHelperInfo is the answer to CmdQuestHelper.
type QuestHelperInfo struct {
	// Active: the Quest Helper plugin is on in the client.
	Active bool `json:"active"`
	// Selected is the quest it follows now ("" when none), and Running
	// whether its bot drives that quest now ("Enable QuestHelper" on).
	Selected string `json:"selected,omitempty"`
	Running  bool   `json:"running"`
	// Quests are the names it lists (/quest-helper/list): what
	// /quest-helper/start takes.
	Quests []string `json:"quests"`
}

// maxQuestNames bounds the names a client's list may give (the pinned
// client lists 276 helpers).
const maxQuestNames = 2000

// questStatus is the Agent Server's GET /quest-helper/status.
type questStatus struct {
	Active bool `json:"active"`
	// TurnOn is "Enable QuestHelper" as the client stores it ("true",
	// "false"); absent means its default, on.
	TurnOn    string `json:"turnOn"`
	Selected  bool   `json:"selected"`
	QuestName string `json:"questName"`
	// Completed is the helper's own reading of the quest's vars: the
	// trail's, never the proof.
	Completed bool   `json:"completed"`
	StepText  string `json:"stepText"`
}

// botOn reports whether the bot's switch ("Enable QuestHelper") is on.
func (s questStatus) botOn() bool { return !strings.EqualFold(strings.TrimSpace(s.TurnOn), "false") }

// driving reports whether the bot drives a quest now.
func (s questStatus) driving() bool { return s.Active && s.Selected && s.botOn() }

// errNoQuestHelper: the client has no Quest Helper the agent can drive (no
// /quest-helper endpoints, or no Quest Helper plugin).
var errNoQuestHelper = errors.New("the client has no Quest Helper the Agent Server drives (/quest-helper)")

// questStatus reads GET /quest-helper/status.
func (e *Executor) questStatus(ctx context.Context) (questStatus, error) {
	var s questStatus
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/quest-helper/status", nil, nil, 10*time.Second)
	if err != nil {
		return s, questErr(err)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("the Quest Helper's status: %w", err)
	}
	return s, nil
}

// questNames reads GET /quest-helper/list: the names it lists, the
// printable one-line ones only.
func (e *Executor) questNames(ctx context.Context) ([]string, error) {
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/quest-helper/list", nil, nil, 15*time.Second)
	if err != nil {
		return nil, questErr(err)
	}
	var l struct {
		Quests []struct {
			DisplayName string `json:"displayName"`
		} `json:"quests"`
	}
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fmt.Errorf("the Quest Helper's list: %w", err)
	}
	out := []string{}
	for _, q := range l.Quests {
		n := q.DisplayName
		if n == "" || len(n) > execspec.MaxName || strings.IndexFunc(n, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 || slices.Contains(out, n) {
			continue
		}
		if out = append(out, n); len(out) >= maxQuestNames {
			break
		}
	}
	return out, nil
}

// questErr names a client without the Quest Helper's endpoints, or
// without the plugin (both answer 404).
func questErr(err error) error {
	var api *microbot.APIError
	if errors.As(err, &api) && api.Status == http.StatusNotFound {
		return fmt.Errorf("%w: %s", errNoQuestHelper, clip(api.Message, 200))
	}
	return err
}

// questHelper serves CmdQuestHelper.
func (e *Executor) questHelper(ctx context.Context) (QuestHelperInfo, error) {
	st, err := e.questStatus(ctx)
	if err != nil {
		return QuestHelperInfo{}, err
	}
	names, err := e.questNames(ctx)
	if err != nil {
		return QuestHelperInfo{}, err
	}
	info := QuestHelperInfo{Active: st.Active, Running: st.driving(), Quests: names}
	if st.Selected {
		info.Selected = clip(st.QuestName, execspec.MaxName)
	}
	return info, nil
}

// setQuestHelper writes one of the Quest Helper's settings through the
// config command's own request (CmdConfigSet: its checks, the pinned
// values included) and verifies the client kept it.
func (e *Executor) setQuestHelper(ctx context.Context, key, value string) error {
	params, _ := json.Marshal(ConfigSetParams{Group: clientcfg.QuestHelperGroup, Key: key, Value: &value})
	out, err := e.plugin(ctx, CmdConfigSet, params)
	if err != nil {
		return fmt.Errorf("setting %s.%s: %w", clientcfg.QuestHelperGroup, key, err)
	}
	raw, _ := out.(json.RawMessage)
	var got struct {
		Value *string `json:"value"`
	}
	if json.Unmarshal(raw, &got) != nil || got.Value == nil || *got.Value != value {
		return fmt.Errorf("the client did not keep %s.%s = %s", clientcfg.QuestHelperGroup, key, value)
	}
	return nil
}

// stopQuest stops the Quest Helper's quest (POST /quest-helper/stop: the
// helper drops it, and its bot has nothing to drive) and, when the run
// switched the bot on, switches it off again; whatever ended the run, a
// cancel included.
func (e *Executor) stopQuest(restoreOff bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := e.Game.JSON(ctx, http.MethodPost, "/quest-helper/stop", nil, map[string]any{}, 30*time.Second)
	if restoreOff {
		if rerr := e.setQuestHelper(ctx, clientcfg.QuestTurnOn, "false"); err == nil {
			err = rerr
		}
	}
	return err
}

// questWork is the Quest Helper's bot as a wait_until run supervises it:
// working while the helper is on, follows this quest, has not judged it
// completed and the bot's switch is on (it switches itself off when it gives
// up); its active step on the trail.
func (e *Executor) questWork(q execspec.Quest) untilWork {
	var last questStatus
	step, noted := "", 0
	return untilWork{
		name: simple(clientcfg.QuestHelperClass),
		running: func(ctx context.Context) (bool, bool) {
			st, err := e.questStatus(ctx)
			if err != nil {
				return false, false
			}
			last = st
			return st.driving() && st.QuestName == q.Helper && !st.Completed, true
		},
		stopped: func(context.Context) error {
			why := "o bot desligou \"Enable QuestHelper\" (desistiu)"
			switch {
			case !last.Active:
				why = "o plugin Quest Helper foi desligado"
			case !last.Selected:
				why = "o Quest Helper largou a quest"
			case last.QuestName != q.Helper:
				why = "o Quest Helper passou a seguir " + clip(last.QuestName, execspec.MaxName)
			case last.Completed:
				why = "o Quest Helper diz que a quest terminou (a prova é do ingest)"
			}
			e.note(RunRunning, why)
			return nil
		},
		progress: func(context.Context, int) {
			text := strings.Join(strings.Fields(last.StepText), " ")
			if text == "" || text == step || noted >= maxTaskEvents {
				return
			}
			step = text
			noted++
			e.note(RunRunning, "passo do Quest Helper: "+clip(text, 300))
		},
	}
}

// driveQuest is a quest run's goroutine: the safe point and the checks,
// "Obtain Missing Items" to No, the quest selected, then the helper's bot
// supervised until the proof, the timeout, a failure or the owner's cancel,
// which stop it.
func (e *Executor) driveQuest(ctx context.Context, p TaskRunParams) {
	q := *p.Task.Quest
	until := RunUntil{Objective: q.Until(), Account: p.Account}
	fail := func(err error) {
		if ctx.Err() != nil {
			e.end(RunCanceled, "cancelado pelo dono; a quest parou onde estava", "")
			return
		}
		e.end(RunFailed, err.Error()+"; a quest parou onde estava", err.Error())
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
		fail(errors.New("the character is in the wilderness: an HCIM quests there only with the owner watching"))
		return
	}
	if e.untilMet() {
		done()
		return
	}
	// The quest's end is proven from Chronicle Link's tracking (its quest
	// states): without it the run would end only on the timeout.
	if v := e.bridgeVersion(ctx); v == "" {
		fail(errors.New("the Chronicle Link bridge did not answer: without its tracking the ingest cannot prove the quest"))
		return
	}
	hs, err := e.questStatus(ctx)
	switch {
	case err != nil:
		fail(err)
		return
	case !hs.Active:
		fail(errors.New("the Quest Helper plugin is off in the client: switch it on (the run selects the quest, it does not start the plugin)"))
		return
	case hs.driving():
		fail(fmt.Errorf("the Quest Helper's bot is already doing %s: stop it first, the quest run selects the quest and stops it", clip(hs.QuestName, execspec.MaxName)))
		return
	}
	names, err := e.questNames(ctx)
	if err != nil {
		fail(err)
		return
	}
	if !slices.Contains(names, q.Helper) {
		fail(fmt.Errorf("the client's Quest Helper does not list %q", q.Helper))
		return
	}
	e.note(RunRunning, fmt.Sprintf("em %d, %d, %d; %s pelo Quest Helper até o ingest provar %s", at.X, at.Y, at.Plane, q.Name, until.Objective))
	var others []string
	for _, s := range e.activeScripts(ctx) {
		if s != simple(clientcfg.QuestHelperClass) {
			others = append(others, s)
		}
	}
	if len(others) > 0 {
		// Only a warning, as for a train: a main activity left on would
		// click alongside the quest.
		e.note(RunRunning, "plugins ligados durante a quest: "+strings.Join(others, ", ")+" (pare a atividade principal antes)")
	}
	// Ironman: the bot never fetches items (the bank nor the Grand
	// Exchange); a bank task before the quest brings them.
	if err = e.setQuestHelper(ctx, clientcfg.QuestObtainMissingItems, clientcfg.QuestObtainNo); err != nil {
		fail(err)
		return
	}
	e.note(RunRunning, "configuração: "+clientcfg.QuestHelperGroup+"."+clientcfg.QuestObtainMissingItems+" = "+clientcfg.QuestObtainNo+" (o bot nunca busca itens no banco nem no Grand Exchange)")
	if _, err = e.Game.JSON(ctx, http.MethodPost, "/quest-helper/start", nil, map[string]any{"name": q.Helper}, 30*time.Second); err != nil {
		_ = e.stopQuest(false)
		fail(fmt.Errorf("starting %s in the Quest Helper: %w", q.Helper, questErr(err)))
		return
	}
	// /quest-helper/start switches the bot on in the RuneScape profile's
	// settings, which the bot does not read: after the bot gave up once
	// ("Enable QuestHelper" off), it is switched on here, and off again at
	// the end.
	turnedOn := false
	if !hs.botOn() {
		if err = e.setQuestHelper(ctx, clientcfg.QuestTurnOn, "true"); err != nil {
			_ = e.stopQuest(false)
			fail(err)
			return
		}
		turnedOn = true
		e.note(RunRunning, "\"Enable QuestHelper\" ligado (estava desligado; volta a desligado no fim)")
	}
	e.note(RunRunning, fmt.Sprintf("iniciado: %s no Quest Helper, até o ingest provar %s", q.Name, until.Objective))
	timeout := untilTimeout
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	if e.taskTimeout > 0 {
		timeout = e.taskTimeout
	}
	if _, err = e.superviseUntil(ctx, e.questWork(q), until, timeout); err != nil {
		_ = e.stopQuest(turnedOn)
		fail(err)
		return
	}
	if err = e.stopQuest(turnedOn); err != nil {
		fail(fmt.Errorf("stopping the Quest Helper after the proof: %w", err))
		return
	}
	e.note(RunRunning, "parado: Quest Helper (objetivo cumprido)")
	e.end(RunDone, "quest feita: "+until.Objective+" provado pelo ingest", "")
}
