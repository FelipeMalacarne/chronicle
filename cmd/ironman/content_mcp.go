package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/config"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/annotate"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/mcp"
)

func newContentMCPCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve the guide extraction tools over MCP (stdio, offline)",
		Long: `Serve the guide extraction tools to an agent over the Model Context Protocol
on stdin/stdout (docs/09 section 6). It works only on the content directory:
no database, no network. It writes the annotation sidecars, decisions.json,
annotations.report.json and owner-confirmed unlock/location entries; the
agent never edits those files itself. Declared for Claude Code in .mcp.json
and driven by the annotate-guide skill.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				dir = cfg.ContentDir
			}
			ws, err := annotate.Open(dir)
			if err != nil {
				return err
			}
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			srv := newContentMCPServer(ws)
			srv.Log = log
			log.Info("content mcp ready", "content_dir", dir)
			return srv.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&dir, "content-dir", "", `content directory (default: $CONTENT_DIR or "content")`)
	return cmd
}

const contentMCPInstructions = `Guide extraction for the Ironman Planner (docs/09 section 6). Follow the
annotate-guide skill. Loop per section: status -> next_batch -> resolve_entity
for names you are unsure of -> submit -> fix and resubmit rejected steps.
Send names and quotes, never refs or spans: submit resolves names and locates
quotes. Anything inferred (confidence medium) needs the owner's answer first
(confirmed_by "owner") or a recorded decision. New unlocks and locations enter
the catalogs only through propose_entity after the owner confirms.`

func newContentMCPServer(ws *annotate.Workspace) *mcp.Server {
	srv := &mcp.Server{
		Name:         "ironman-content",
		Version:      buildinfo.Version,
		Instructions: contentMCPInstructions,
		Tools: []mcp.Tool{
			{
				Name:        "status",
				Description: "Annotation coverage per guide and section: steps annotated, stale (text changed since) and pending. Omit guide for every guide.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"properties":{"guide":{"type":"string","description":"guide id, e.g. oziris-main"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide string `json:"guide"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.Status(a.Guide)
				},
			},
			{
				Name: "next_batch",
				Description: "The next steps of a section without a current annotation, in guide order, with their quotable fields (text, location, notes, items), " +
					"entity mentions the importers resolved, objectives already converted from rules, the segment (and its derived loadout/bank actions when the step starts it) " +
					"and the owner's decisions for the guide.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["guide","section"],"properties":{
					"guide":{"type":"string"},"section":{"type":"string","description":"section id (1.1) or slug"},
					"limit":{"type":"integer","minimum":1,"maximum":50,"description":"max steps (default 8; long steps end a batch earlier)"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide   string `json:"guide"`
						Section string `json:"section"`
						Limit   int    `json:"limit"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.NextBatch(a.Guide, a.Section, a.Limit)
				},
			},
			{
				Name: "resolve_entity",
				Description: "Resolve a name the way submit will: resolved (ref), ambiguous (candidates: pass the chosen one as id, or use any_of/family for items) or unresolved (with reason). " +
					"Kinds: quest, skill, diary (name = area, plus tier), boss, item, unlock, location, ca. For items, family is set when the name covers a whole family.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["kind","name"],"properties":{
					"kind":{"type":"string","enum":["quest","skill","diary","boss","item","unlock","location","ca"]},
					"name":{"type":"string"},"tier":{"type":"string","enum":["easy","medium","hard","elite"]}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Kind string `json:"kind"`
						Name string `json:"name"`
						Tier string `json:"tier"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.ResolveEntity(a.Kind, a.Name, a.Tier), nil
				},
			},
			{
				Name: "propose_entity",
				Description: "Add a new unlock or location to content/reference after the owner confirmed it (ask first). Refused when the name already resolves. " +
					"The next submit resolves it.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["kind","name","confirmed_by"],"properties":{
					"kind":{"type":"string","enum":["unlock","location"]},
					"id":{"type":"string","description":"lower-case slug; default: slug of name"},
					"name":{"type":"string"},"wiki_key":{"type":"string","description":"wiki page title"},
					"aliases":{"type":"array","items":{"type":"string"}},"note":{"type":"string"},
					"confirmed_by":{"const":"owner"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var p annotate.EntityProposal
					if err := mcp.Decode(raw, &p); err != nil {
						return nil, err
					}
					return ws.ProposeEntity(p)
				},
			},
			{
				Name: "revise_entity",
				Description: "Rename, edit or remove an unlock or location entry after the owner confirmed it (ask first). new_id renames the entry and rewrites its refs in every sidecar; " +
					"name, wiki_key, aliases and note replace the old values when given (an empty wiki_key or note clears it); remove is refused while a sidecar or rule still uses the entry.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["kind","id","confirmed_by"],"properties":{
					"kind":{"type":"string","enum":["unlock","location"]},
					"id":{"type":"string","description":"current id (slug)"},
					"new_id":{"type":"string","description":"new lower-case slug"},
					"name":{"type":"string"},"wiki_key":{"type":"string","description":"wiki page title; empty clears it"},
					"aliases":{"type":"array","items":{"type":"string"},"description":"replaces every alias"},"note":{"type":"string"},
					"remove":{"type":"boolean"},
					"confirmed_by":{"const":"owner"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var rv annotate.EntityRevision
					if err := mcp.Decode(raw, &rv); err != nil {
						return nil, err
					}
					return ws.ReviseEntity(rv)
				},
			},
			{
				Name: "submit",
				Description: "Verify and store annotations for steps of one section. Each step is accepted whole or rejected with every reason " +
					"(name does not resolve or is ambiguous, quote not in the field, inference without the owner's confirmation, wrong role, route order). " +
					"Accepted steps are written to the section sidecar; resubmitting a step replaces its annotation. Submit a step with no objectives to record that it has none. " +
					"A rule objective the step does not have stays in force unless it is listed in suppress (with the owner's confirmation).",
				InputSchema: schema(submitSchema),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide        string                  `json:"guide"`
						Section      string                  `json:"section"`
						SkillVersion string                  `json:"skill_version"`
						Steps        []annotate.StepProposal `json:"steps"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.Submit(a.Guide, a.Section, a.SkillVersion, a.Steps)
				},
			},
			{
				Name:        "record_decision",
				Description: "Record an owner's answer that holds as a general rule (scope says where), to apply in later batches without asking again; cite it with source.decision.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["guide","id","question","answer","scope"],"properties":{
					"guide":{"type":"string"},"id":{"type":"string","description":"lower-case slug, e.g. axe-means-best-axe"},
					"question":{"type":"string"},"answer":{"type":"string"},"scope":{"type":"string"},
					"step_ids":{"type":"array","items":{"type":"string"},"description":"steps where it was asked"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide string `json:"guide"`
						content.Decision
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.RecordDecision(a.Guide, a.Decision)
				},
			},
			{
				Name:        "list_decisions",
				Description: "The owner's recorded decisions for a guide.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["guide"],"properties":{"guide":{"type":"string"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide string `json:"guide"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.Decisions(a.Guide)
				},
			},
			{
				Name: "report",
				Description: "The extraction report of a guide (also written to annotations.report.json): coverage and first-submit acceptance per section, " +
					"agreement with the rule-derived objectives, steps whose last submit was rejected, and everything that entered with medium confidence.",
				InputSchema: schema(`{"type":"object","additionalProperties":false,"required":["guide"],"properties":{"guide":{"type":"string"}}}`),
				Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
					var a struct {
						Guide string `json:"guide"`
					}
					if err := mcp.Decode(raw, &a); err != nil {
						return nil, err
					}
					return ws.Report(a.Guide)
				},
			},
		},
	}
	// Every tool first picks up catalogs changed on disk since the last call.
	for i, t := range srv.Tools {
		h := t.Handler
		srv.Tools[i].Handler = func(ctx context.Context, raw json.RawMessage) (any, error) {
			if err := ws.SyncRegistry(); err != nil {
				return nil, err
			}
			return h(ctx, raw)
		}
	}
	return srv
}

func schema(s string) json.RawMessage {
	if !json.Valid([]byte(s)) {
		panic("content mcp: invalid tool schema: " + s)
	}
	return json.RawMessage(s)
}

// submitSchema describes annotate.StepProposal.
const submitSchema = `{"type":"object","additionalProperties":false,"required":["guide","section","skill_version","steps"],
"$defs":{
 "quote":{"type":"string","minLength":1,"description":"exact words of the step field (case, spaces and curly quotes may differ)"},
 "field":{"type":"string","description":"JSON pointer in the step: /text (default), /location, /notes/<i>/text, /items/<i>"},
 "confidence":{"type":"string","enum":["high","medium"],"description":"high: the text says it; medium: inferred, needs confirmed_by or decision"},
 "confirmed_by":{"const":"owner"},
 "decision":{"type":"string","description":"id of a recorded decision this applies"}
},
"properties":{
 "guide":{"type":"string"},"section":{"type":"string"},
 "skill_version":{"type":"string","description":"annotate-guide@<version> from the skill"},
 "steps":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["step_id"],"properties":{
  "step_id":{"type":"string"},
  "comment":{"type":"string","description":"report only: why no objectives, why it diverges from a rule, what was asked"},
  "objectives":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["role","kind","quote","confidence"],"properties":{
   "role":{"type":"string","enum":["completes","starts","requires","uses"],"description":"starts only for quest started; uses only for items"},
   "kind":{"type":"string","enum":["quest","skill","diary","boss","item","unlock","ca","qp"]},
   "name":{"type":"string","description":"entity name; the area for a diary; omit for qp"},
   "id":{"type":"string","description":"chosen candidate id when the name is ambiguous"},
   "tier":{"type":"string","enum":["easy","medium","hard","elite"]},
   "state":{"type":"string","enum":["started","finished"],"description":"quest only"},
   "n":{"type":"integer","minimum":1,"description":"skill level, boss kill count, item quantity, quest points"},
   "field":{"$ref":"#/$defs/field"},"quote":{"$ref":"#/$defs/quote"},
   "confidence":{"$ref":"#/$defs/confidence"},"confirmed_by":{"$ref":"#/$defs/confirmed_by"},"decision":{"$ref":"#/$defs/decision"}}}},
  "items":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["name","quote","confidence"],"properties":{
   "name":{"type":"string","description":"item as the step names it, without quantity"},
   "id":{"type":"string","description":"chosen candidate id of an ambiguous name"},
   "any_of":{"type":"array","items":{"type":"string"},"description":"item names (or candidate ids of name), preference order; each must reach qty alone"},
   "family":{"type":"string","description":"family name or slug: every dose/charge counts"},
   "qty":{"type":"integer","minimum":1,"description":"interpreted: 1k = 1000"},
   "noted":{"type":"boolean"},"optional":{"type":"boolean"},
   "slot":{"type":"string","enum":["inventory","equipment"]},
   "field":{"$ref":"#/$defs/field"},"quote":{"$ref":"#/$defs/quote"},
   "confidence":{"$ref":"#/$defs/confidence"},"confirmed_by":{"$ref":"#/$defs/confirmed_by"},"decision":{"$ref":"#/$defs/decision"}}}},
  "bank_actions":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["type","position","quote","confidence"],"properties":{
   "type":{"type":"string","enum":["open_bank","deposit_all","deposit","withdraw","bank_setup","close_bank"]},
   "position":{"type":"string","enum":["start","within","end"]},
   "field":{"$ref":"#/$defs/field"},"quote":{"$ref":"#/$defs/quote"},
   "confidence":{"$ref":"#/$defs/confidence"},"confirmed_by":{"$ref":"#/$defs/confirmed_by"},"decision":{"$ref":"#/$defs/decision"}}}},
  "location":{"type":"object","additionalProperties":false,"required":["name","quote","confidence"],"properties":{
   "name":{"type":"string"},"id":{"type":"string"},
   "field":{"$ref":"#/$defs/field"},"quote":{"$ref":"#/$defs/quote"},
   "confidence":{"$ref":"#/$defs/confidence"},"confirmed_by":{"$ref":"#/$defs/confirmed_by"},"decision":{"$ref":"#/$defs/decision"}}},
  "suppress":{"type":"array","description":"rule objectives the step does not have (the loader drops them); always needs the owner","items":{"type":"object","additionalProperties":false,"required":["key","role","reason"],"properties":{
   "key":{"type":"string","description":"key of one of the step's rule_objectives, as next_batch lists it"},
   "role":{"type":"string","enum":["completes","starts","requires","uses"]},
   "reason":{"type":"string","minLength":1,"description":"why the step does not have it"},
   "confirmed_by":{"$ref":"#/$defs/confirmed_by"},"decision":{"$ref":"#/$defs/decision"}}}}
 }}}
}}`
