package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/mcp"
)

// newMCPCmd is `chronicle mcp`: the account's progress for agents, over
// stdio, as a client of the API like the CLI (docs/04 section 2.2). It is
// distinct from `chronicle content mcp`, which works offline over content/.
func newMCPCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the account MCP server over stdio (client of the API, like the CLI)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := f.connect(cmd.Context())
			if err != nil {
				return err
			}
			srv := newAccountMCPServer(c)
			srv.Log = slog.New(slog.NewTextHandler(os.Stderr, nil))
			return srv.Serve(cmd.Context(), os.Stdin, os.Stdout)
		},
	}
	f.add(cmd, false)
	return cmd
}

const accountMCPInstructions = `Progress of one OSRS Ironman account in the Chronicle planner, which follows
independent routes (Oziris Ironman Efficiency Guide, BRUHsailer). Start with
now: the current step of the active route, its objectives with the account's
verdicts (met, unmet, unknown: unknown is never "not done"), what to bring
and the next steps. Steps are done by evidence (hiscores, plugin); mark_step
records the owner's own marks, so call it only when the owner asks. Keep the
attribution line of the route when you show its steps.`

func newAccountMCPServer(c *client.Client) *mcp.Server {
	noArgs := schema(`{"type":"object","additionalProperties":false,"properties":{}}`)
	empty := func(raw json.RawMessage) error { return mcp.Decode(raw, &struct{}{}) }
	srv := &mcp.Server{
		Name:         "chronicle",
		Version:      buildinfo.Version,
		Instructions: accountMCPInstructions,
		Tools: []mcp.Tool{
			{
				Name:        "now",
				Description: "The active route from the current step: the step (text, notes, objectives with verdicts, what blocks it), what to bring for its leg and the next steps, with the route's attribution.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"properties":{"limit":{"type":"integer","minimum":1,"maximum":50,"description":"steps after the current one (default 15)"}}}`),
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Limit int `json:"limit"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return c.Route(ctx, a.Limit)
				},
			},
			{
				Name:        "mark_step",
				Description: "Record the owner's mark on a step of the active route (only when the owner asks): done, skip, pin (keep open) or undo. Without step, the current step. Returns the route from the new current step.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["action"],"properties":{
 "action":{"type":"string","enum":["done","skip","pin","undo"]},
 "step":{"type":"string","description":"step ref as now shows it, or step id; default the current step"},
 "note":{"type":"string","description":"note on the step (empty clears it)"}}}`),
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Action string  `json:"action"`
						Step   string  `json:"step"`
						Note   *string `json:"note"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					action, ok := map[string]coreprogress.MarkAction{
						"done": coreprogress.ActionManualDone, "skip": coreprogress.ActionSkipped,
						"pin": coreprogress.ActionPinnedOpen, "undo": coreprogress.ActionClear,
					}[a.Action]
					if !ok {
						return nil, fmt.Errorf("unknown action %q", a.Action)
					}
					w, err := c.Route(ctx, panel.MaxLimit)
					if err != nil {
						return nil, err
					}
					var args []string
					if a.Step != "" {
						args = []string{a.Step}
					}
					step, err := pickStep(w, args)
					if err != nil {
						return nil, err
					}
					if err := c.Mark(ctx, w.Guide, step.ID, action, a.Note); err != nil {
						return nil, err
					}
					return c.Route(ctx, 5)
				},
			},
			{
				Name:        "goals",
				Description: "The account's goals (pinned objectives) with their verdicts.",
				InputSchema: noArgs,
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					if err := empty(raw); err != nil {
						return nil, err
					}
					return c.Goals(ctx)
				},
			},
			{
				Name:        "goal_plan",
				Description: "The ordered plan of one goal: requirements met and missing, level bottlenecks, suggested order and where each route does it.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["goal"],"properties":{"goal":{"type":"string","description":"goal id, or a unique piece of its name"}}}`),
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Goal string `json:"goal"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					goals, err := c.Goals(ctx)
					if err != nil {
						return nil, err
					}
					g, err := pickGoal(goals, a.Goal)
					if err != nil {
						return nil, err
					}
					return c.GoalPlan(ctx, g.ID)
				},
			},
			{
				Name:        "account_state",
				Description: "What the planner knows of the account, one part at a time: summary (default: levels, quest and diary counts, item containers with their freshness), skills, quests, diaries or items (inventory, equipment, bank). search filters quests and items by name.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"properties":{
 "part":{"type":"string","enum":["summary","skills","quests","diaries","items"],"description":"default summary"},
 "search":{"type":"string","description":"case-insensitive piece of a quest or item name (or a quest state: FINISHED, IN_PROGRESS, NOT_STARTED)"}}}`),
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Part   string `json:"part"`
						Search string `json:"search"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					st, err := c.AccountState(ctx)
					if err != nil {
						return nil, err
					}
					return accountStatePart(st, a.Part, a.Search)
				},
			},
			{
				Name:        "step",
				Description: "One step of a guide as the guide writes it (text, notes, items, objectives).",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["guide","id"],"properties":{"guide":{"type":"string","description":"guide id, e.g. oziris-main"},"id":{"type":"string","description":"step id"}}}`),
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide string `json:"guide"`
						ID    string `json:"id"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return c.Get(ctx, "/api/guides/"+url.PathEscape(a.Guide)+"/steps/"+url.PathEscape(a.ID), nil, false)
				},
			},
			{
				Name:        "sync",
				Description: "Sync the account from the hiscores now (rate limited by the server).",
				InputSchema: noArgs,
				Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
					if err := empty(raw); err != nil {
						return nil, err
					}
					return c.Sync(ctx)
				},
			},
		},
	}
	// The model sees the same explanations as the CLI user.
	for i, t := range srv.Tools {
		h := t.Handler
		srv.Tools[i].Handler = func(ctx context.Context, raw json.RawMessage) (any, error) {
			v, err := h(ctx, raw)
			return v, explain(err)
		}
	}
	return srv
}
