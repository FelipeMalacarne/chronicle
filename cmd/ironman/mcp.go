package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
	"github.com/FelipeMalacarne/ironman-planner/internal/mcp"
)

func newAccountMCPCmd() *cobra.Command {
	var baseURL, account string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve the account tools (route, marks, goals, graph, state, sync) over MCP on stdio",
		Long: `Serve the account's planner to an agent over the Model Context Protocol on
stdin/stdout: the current step and the next ones, marks, goals and their
plans, the requirement graph, the account state and a hiscores sync. Every
tool calls the API of a running server (IRONMAN_URL, IRONMAN_TOKEN); it never
opens the database. Marks are recorded with via=mcp.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			srv := newAccountMCPServer(client.New(baseURL, os.Getenv("IRONMAN_TOKEN"), client.ViaMCP), account)
			srv.Log = log
			log.Info("account mcp ready", "url", baseURL)
			return srv.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	def := os.Getenv("IRONMAN_URL")
	if def == "" {
		def = client.DefaultURL
	}
	cmd.Flags().StringVar(&baseURL, "url", def, "server base URL (IRONMAN_URL)")
	cmd.Flags().StringVar(&account, "account", os.Getenv("IRONMAN_ACCOUNT"), "account id (IRONMAN_ACCOUNT; default: the user's only account)")
	return cmd
}

const accountMCPInstructions = `The owner's OSRS Ironman planner (Chronicle). Two independent routes
(oziris-main, bruhsailer-main); the account follows one, the active route.
Start with "now" for the current step and what to bring. Progress is detected
from the game (plugin) and the hiscores: an objective is met only with
evidence, and "?" (unknown) never means not done. Mark a step only when the
owner says they did, skipped or want it open: mark_step writes the owner's
mark. Goals are objective keys (quest:legends_quest@finished,
skill:agility>=50, qp>=175, item:<id>); "graph" explains what a key needs
and unlocks. Text tools answer in PT-BR, the owner's language; pass
format "json" for the API's JSON.`

// accountTool is a tool whose handler gets the client, the account and the
// raw arguments (the account resolved per call: the server may start later).
type accountTool struct {
	name, description, schema string
	run                       func(ctx context.Context, c *client.Client, account string, args json.RawMessage) (any, error)
}

func newAccountMCPServer(c *client.Client, account string) *mcp.Server {
	tools := []accountTool{
		{
			name:        "now",
			description: "The current step of the active route with its text, objectives and notes, the items to bring for its leg, and the next steps (blocked ones say what is missing).",
			schema:      `{"type":"object","additionalProperties":false,"properties":{"limit":{"type":"integer","minimum":1,"maximum":50,"description":"steps after the current one (default 5)"},"format":{"type":"string","enum":["text","json"]}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Limit  int    `json:"limit"`
					Format string `json:"format"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				if a.Limit == 0 {
					a.Limit = 5
				}
				w, err := c.Route(ctx, account, a.Limit)
				if err != nil {
					return nil, err
				}
				return render(a.Format, w, func(out io.Writer) { printWindow(out, w) }), nil
			},
		},
		{
			name:        "routes",
			description: "Every route with its cursor (author's reference), steps done of total, blocked steps and which one is active; plus the account's sources and their last observation.",
			schema:      `{"type":"object","additionalProperties":false,"properties":{}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				if err := mcp.Decode(raw, &struct{}{}); err != nil {
					return nil, err
				}
				p, err := c.Progress(ctx, account)
				if err != nil {
					return nil, err
				}
				type route struct {
					Guide   string `json:"guide"`
					Title   string `json:"title"`
					Cursor  string `json:"cursor"`
					Section string `json:"section"`
					Active  bool   `json:"active"`
					Done    int    `json:"done"`
					Total   int    `json:"total"`
					Blocked int    `json:"blocked"`
				}
				out := struct {
					ActiveGuide string          `json:"active_guide"`
					Routes      []route         `json:"routes"`
					Sources     []client.Source `json:"sources"`
				}{ActiveGuide: p.ActiveGuide, Routes: []route{}, Sources: p.Sources}
				for _, r := range p.Routes {
					out.Routes = append(out.Routes, route{Guide: r.Guide, Title: r.Title, Cursor: r.CursorRef, Section: r.CursorSection, Active: r.Active, Done: r.Counts.Done, Total: r.Counts.Total, Blocked: r.Counts.Blocked})
				}
				return out, nil
			},
		},
		{
			name: "mark_step",
			description: "Write the owner's mark on a step: manual_done (did it), skipped, pinned_open (keep open) or clear (undo the mark). " +
				"Step: id or author's reference (1.1.219, CH1.6); default the current step. Guide: default the active route. Only when the owner says so.",
			schema: `{"type":"object","additionalProperties":false,"required":["kind"],"properties":{
				"kind":{"type":"string","enum":["manual_done","skipped","pinned_open","clear"]},
				"step":{"type":"string"},"guide":{"type":"string"},
				"note":{"type":"string","description":"the step's note; empty clears it"}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Kind  client.MarkAction `json:"kind"`
					Step  string            `json:"step"`
					Guide string            `json:"guide"`
					Note  *string           `json:"note"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				if a.Guide == "" {
					p, err := c.Progress(ctx, account)
					if err != nil {
						return nil, err
					}
					a.Guide = p.ActiveGuide
				}
				before, err := c.Board(ctx, account, a.Guide)
				if err != nil {
					return nil, err
				}
				step, err := pickStep(before, a.Step)
				if err != nil {
					return nil, err
				}
				after, err := c.Mark(ctx, account, a.Guide, step.ID, a.Kind, a.Note)
				if err != nil {
					return nil, err
				}
				return capture(func(out io.Writer) { printMarked(out, step, a.Kind, after) }), nil
			},
		},
		{
			name:        "goals",
			description: "The account's pinned goals with their verdicts and ids.",
			schema:      `{"type":"object","additionalProperties":false,"properties":{"format":{"type":"string","enum":["text","json"]}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Format string `json:"format"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				goals, err := c.Goals(ctx, account)
				if err != nil {
					return nil, err
				}
				return render(a.Format, goals, func(out io.Writer) { printGoals(out, goals) }), nil
			},
		},
		{
			name:        "goal_plan",
			description: "The plan of a goal (id, objective key or name): open requirements in suggested order, level bottlenecks, where each route completes it. pin true pins an objective key first.",
			schema:      `{"type":"object","additionalProperties":false,"required":["goal"],"properties":{"goal":{"type":"string"},"pin":{"type":"boolean"},"label":{"type":"string"},"format":{"type":"string","enum":["text","json"]}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Goal   string `json:"goal"`
					Pin    bool   `json:"pin"`
					Label  string `json:"label"`
					Format string `json:"format"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				id := ""
				if a.Pin {
					g, err := c.AddGoal(ctx, account, a.Goal, a.Label)
					if err != nil {
						return nil, err
					}
					id = g.ID
				} else {
					goals, err := c.Goals(ctx, account)
					if err != nil {
						return nil, err
					}
					g, err := pickGoal(goals, a.Goal)
					if err != nil {
						return nil, err
					}
					id = g.ID
				}
				p, err := c.GoalPlan(ctx, account, id)
				if err != nil {
					return nil, err
				}
				return render(a.Format, p, func(out io.Writer) { printPlan(out, p) }), nil
			},
		},
		{
			name:        "remove_goal",
			description: "Unpin a goal (id, objective key or name).",
			schema:      `{"type":"object","additionalProperties":false,"required":["goal"],"properties":{"goal":{"type":"string"}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Goal string `json:"goal"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				goals, err := c.Goals(ctx, account)
				if err != nil {
					return nil, err
				}
				g, err := pickGoal(goals, a.Goal)
				if err != nil {
					return nil, err
				}
				if err := c.RemoveGoal(ctx, account, g.ID); err != nil {
					return nil, err
				}
				return "Meta removida: " + goalName(g), nil
			},
		},
		{
			name:        "graph",
			description: "Requirement graph of any objective key: requirements by depth with verdicts, the critical path (longest chain of open requirements) and the quests it unlocks.",
			schema:      `{"type":"object","additionalProperties":false,"required":["focus"],"properties":{"focus":{"type":"string","description":"objective key, e.g. quest:legends_quest@finished"},"depth":{"type":"integer","minimum":1,"maximum":6},"all":{"type":"boolean","description":"list done requirements too"},"format":{"type":"string","enum":["text","json"]}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Focus  string `json:"focus"`
					Depth  int    `json:"depth"`
					All    bool   `json:"all"`
					Format string `json:"format"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				g, err := c.Graph(ctx, account, a.Focus, a.Depth)
				if err != nil {
					return nil, err
				}
				return render(a.Format, g, func(out io.Writer) { printGraph(out, g, a.All) }), nil
			},
		},
		{
			name:        "entity",
			description: "One entity (quest:<id>, skill:<name>, item:<id>, diary:<area>, unlock:<id>, location:<id>, qp): its objectives with verdicts, direct requirements, what it unlocks and where each route does it.",
			schema:      `{"type":"object","additionalProperties":false,"required":["ref"],"properties":{"ref":{"type":"string"}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Ref string `json:"ref"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				return c.Entity(ctx, account, a.Ref)
			},
		},
		{
			name:        "account_state",
			description: "The account: skills, quest counts (or every quest), diaries, item containers with freshness and the sources with their age.",
			schema:      `{"type":"object","additionalProperties":false,"properties":{"quests":{"type":"boolean"},"format":{"type":"string","enum":["text","json"]}}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				var a struct {
					Quests bool   `json:"quests"`
					Format string `json:"format"`
				}
				if err := mcp.Decode(raw, &a); err != nil {
					return nil, err
				}
				st, err := c.AccountState(ctx, account)
				if err != nil {
					return nil, err
				}
				p, err := c.Progress(ctx, account)
				if err != nil {
					return nil, err
				}
				return render(a.Format, st, func(out io.Writer) { printState(out, st, p, a.Quests, time.Now()) }), nil
			},
		},
		{
			name:        "sync",
			description: "Sync the hiscores now and reevaluate the account (the server rate-limits it).",
			schema:      `{"type":"object","additionalProperties":false,"properties":{}}`,
			run: func(ctx context.Context, c *client.Client, account string, raw json.RawMessage) (any, error) {
				if err := mcp.Decode(raw, &struct{}{}); err != nil {
					return nil, err
				}
				run, err := c.Sync(ctx, account)
				if err != nil {
					return nil, err
				}
				return capture(func(out io.Writer) { printSync(out, run, time.Now()) }), nil
			},
		},
	}
	srv := &mcp.Server{Name: "ironman", Version: buildinfo.Version, Instructions: accountMCPInstructions}
	for _, t := range tools {
		srv.Tools = append(srv.Tools, mcp.Tool{
			Name: t.name, Description: t.description, InputSchema: schema(t.schema),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				acc, err := c.ResolveAccount(ctx, account)
				if err != nil {
					return nil, err
				}
				return t.run(ctx, c, acc, raw)
			},
		})
	}
	return srv
}

// render is the text rendering, or v itself (JSON) when format is "json".
func render(format string, v any, text func(io.Writer)) any {
	if format == "json" {
		return v
	}
	return capture(text)
}

func capture(f func(io.Writer)) string {
	var buf bytes.Buffer
	f(&buf)
	return buf.String()
}
