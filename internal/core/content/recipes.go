package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/FelipeMalacarne/chronicle/content/schema"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// RecipesSubdir is the directory under the content dir that holds one recipe
// file per objective key (docs/10 section 8).
const RecipesSubdir = "recipes"

// RecipeKind is the kind of a recipe file.
const RecipeKind = "recipe"

// Recipe is content/recipes/<file>.json: how to fulfill one objective key,
// whatever the route. A step that completes a key with a recipe inherits its
// tasks (Content.TasksOf); the step's own tasks are only what is the route's.
type Recipe struct {
	SchemaVersion int                     `json:"schema_version"`
	Kind          string                  `json:"kind"`
	Objective     entities.ObjectiveKey   `json:"objective"`
	Requires      []entities.ObjectiveKey `json:"requires,omitempty"`
	Variants      []RecipeVariant         `json:"variants"`
	Source        ObjectiveSource         `json:"source"`

	// File is the path the recipe was loaded from.
	File string `json:"-"`
}

// RecipeVariant is how the recipe goes for some account modes: a mode is in
// at most one variant (the HCIM variant may take the safe way).
type RecipeVariant struct {
	ID    string   `json:"id"`
	Modes []string `json:"modes"`
	Risks []Risk   `json:"risks,omitempty"`
	Tasks []Task   `json:"tasks"`
}

// Risk is a danger of a recipe variant, from a closed list (docs/10, E9).
type Risk struct {
	Kind     string        `json:"kind"`
	NPC      *entities.Ref `json:"npc,omitempty"`
	Location *entities.Ref `json:"location,omitempty"`
}

// Risk kinds. Combat, wilderness and hazard can kill; item loss cannot.
const (
	RiskCombat     = "combat"
	RiskWilderness = "wilderness"
	RiskHazard     = "hazard"
	RiskItemLoss   = "item_loss"
)

// RiskKinds lists the risk kinds in a stable order.
var RiskKinds = []string{RiskCombat, RiskWilderness, RiskHazard, RiskItemLoss}

// Account modes.
const (
	ModeIronman = "ironman"
	ModeHCIM    = "hcim"
	ModeUIM     = "uim"
	ModeGIM     = "gim"
)

// Modes lists the account modes.
var Modes = []string{ModeIronman, ModeHCIM, ModeUIM, ModeGIM}

// Deadly reports whether the risk can kill the character.
func (r Risk) Deadly() bool { return r.Kind != RiskItemLoss }

// Unsupervised reports whether the variant may run without the owner
// watching for an account of mode: in HCIM nothing with a death risk runs
// unsupervised (docs/10, E9). The executor (phase 3) applies it.
func (v RecipeVariant) Unsupervised(mode string) bool {
	if mode != ModeHCIM {
		return true
	}
	return !slices.ContainsFunc(v.Risks, Risk.Deadly)
}

// VariantFor returns the variant that serves an account mode.
func (r *Recipe) VariantFor(mode string) (*RecipeVariant, bool) {
	for i := range r.Variants {
		if slices.Contains(r.Variants[i].Modes, mode) {
			return &r.Variants[i], true
		}
	}
	return nil, false
}

var nonKeyChars = regexp.MustCompile(`[^a-z0-9_]+`)

// RecipeFileName is the file name of the recipe of key: every run of
// characters outside [a-z0-9_] becomes "-" ("quest:cooks_assistant@finished"
// → "quest-cooks_assistant-finished.json", "skill:agility>=70" →
// "skill-agility-70.json"), which any file system accepts.
func RecipeFileName(k entities.ObjectiveKey) string {
	return strings.Trim(nonKeyChars.ReplaceAllString(k.String(), "-"), "-") + ".json"
}

// loadRecipes reads, validates and verifies every recipe under dir/recipes.
// A missing directory means no recipes.
func loadRecipes(dir string, reg *entities.Registry) (map[string]*Recipe, Errors) {
	files, err := filepath.Glob(filepath.Join(dir, RecipesSubdir, "*.json"))
	if err != nil {
		return nil, Errors{{File: filepath.Join(dir, RecipesSubdir), Message: err.Error()}}
	}
	sort.Strings(files)
	out := map[string]*Recipe{}
	var issues Errors
	for _, f := range files {
		r, fileIssues := LoadRecipeFile(f, reg)
		if len(fileIssues) > 0 {
			issues = append(issues, fileIssues...)
			continue
		}
		key := r.Objective.String()
		if prev, dup := out[key]; dup {
			issues = append(issues, Issue{File: f, Pointer: "/objective", Message: fmt.Sprintf("objective %s already has a recipe in %s", key, prev.File)})
			continue
		}
		out[key] = r
	}
	return out, issues
}

// LoadRecipeFile validates one recipe file against the recipe schema,
// decodes it and verifies it (CheckRecipe), including its file name.
func LoadRecipeFile(path string, reg *entities.Registry) (*Recipe, Errors) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Errors{{File: path, Message: err.Error()}}
	}
	if err := ValidateRecipeJSON(path, data); err != nil {
		if es, ok := AsErrors(err); ok {
			return nil, es
		}
		return nil, Errors{{File: path, Message: err.Error()}}
	}
	var r Recipe
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&r); err != nil {
		return nil, Errors{{File: path, Message: "decode: " + err.Error()}}
	}
	r.File = path
	var issues Errors
	if want := RecipeFileName(r.Objective); filepath.Base(path) != want {
		issues = append(issues, Issue{File: path, Pointer: "/objective", Message: fmt.Sprintf("the recipe of %s must live in %s", r.Objective, want)})
	}
	for _, is := range CheckRecipe(&r, reg) {
		issues = append(issues, Issue{File: path, Pointer: is.Pointer, Message: is.Message})
	}
	return &r, issues
}

// CheckRecipe verifies a recipe (docs/10 sections 7 and 8): the objective and
// every requirement are keys the catalogs know; variant ids are unique and a
// mode is in at most one variant; risks are of the closed list with known
// refs; every task passes Task.Check in the recipe's context; the last task
// of each variant ends on the recipe's objective, so a recipe that ran is a
// recipe whose objective the engine can see met; and an inference was
// confirmed by the owner. Pointers are relative to the recipe file.
func CheckRecipe(r *Recipe, reg *entities.Registry) Errors {
	var issues Errors
	add := func(ptr, format string, args ...any) {
		issues = append(issues, Issue{Pointer: ptr, Message: fmt.Sprintf(format, args...)})
	}
	if msg := reg.KnownObjective(r.Objective); msg != "" {
		add("/objective", "%s", msg)
	}
	for i, k := range r.Requires {
		if msg := reg.KnownObjective(k); msg != "" {
			add(fmt.Sprintf("/requires/%d", i), "%s", msg)
		}
		if k == r.Objective {
			add(fmt.Sprintf("/requires/%d", i), "a recipe cannot require its own objective")
		}
	}
	if r.Source.Confidence != ConfidenceHigh && r.Source.ConfirmedBy != ConfirmedByOwner {
		add("/source", "confidence %q is an inference: it needs confirmed_by owner", r.Source.Confidence)
	}
	if r.Source.Decision != "" {
		add("/source/decision", "a recipe belongs to no guide, so it cites no decision")
	}
	if len(r.Variants) == 0 {
		add("/variants", "a recipe needs a variant")
	}
	ctx := TaskContext{Goal: []entities.ObjectiveKey{r.Objective}, Recipe: true}
	ids := map[string]bool{}
	modes := map[string]string{}
	for vi, v := range r.Variants {
		vPtr := fmt.Sprintf("/variants/%d", vi)
		if ids[v.ID] {
			add(vPtr+"/id", "duplicate variant id %q", v.ID)
		}
		ids[v.ID] = true
		if len(v.Modes) == 0 {
			add(vPtr+"/modes", "a variant serves at least one mode")
		}
		for mi, m := range v.Modes {
			switch prev, dup := modes[m]; {
			case !slices.Contains(Modes, m):
				add(fmt.Sprintf("%s/modes/%d", vPtr, mi), "unknown mode %q", m)
			case dup:
				add(fmt.Sprintf("%s/modes/%d", vPtr, mi), "mode %s is already served by variant %q", m, prev)
			default:
				modes[m] = v.ID
			}
		}
		for ri, rk := range v.Risks {
			rPtr := fmt.Sprintf("%s/risks/%d", vPtr, ri)
			if !slices.Contains(RiskKinds, rk.Kind) {
				add(rPtr+"/kind", "unknown risk %q (one of %s)", rk.Kind, strings.Join(RiskKinds, ", "))
			}
			if rk.NPC != nil && (rk.NPC.Kind != entities.KindNPC || !reg.Known(*rk.NPC)) {
				add(rPtr+"/npc", "unknown npc %s", rk.NPC)
			}
			if rk.Location != nil && (rk.Location.Kind != entities.KindLocation || !reg.Known(*rk.Location)) {
				add(rPtr+"/location", "unknown location %s", rk.Location)
			}
		}
		if len(v.Tasks) == 0 {
			add(vPtr+"/tasks", "a variant needs tasks")
			continue
		}
		for ti, t := range v.Tasks {
			for _, msg := range t.Check(reg, ctx) {
				add(vPtr+taskPointer(ti), "%s", msg)
			}
		}
		if u, ok := v.Tasks[len(v.Tasks)-1].Ends(); !ok || !u.endsOn(r.Objective) {
			add(fmt.Sprintf("%s%s/until", vPtr, taskPointer(len(v.Tasks)-1)), "the last task must end on the recipe's objective %s (goal_met or that key), not %s", r.Objective, u)
		}
	}
	return issues
}

// Recipe returns the recipe of an objective key.
func (c *Content) Recipe(k entities.ObjectiveKey) (*Recipe, bool) {
	r, ok := c.Recipes[k.String()]
	return r, ok
}

// TasksOf returns the tasks of a step for an account mode: for each objective
// the step completes that has a recipe, the tasks of the recipe's variant for
// mode, in the step's objective order; then the step's own tasks (docs/10
// section 8: the step's tasks are only what is the route's).
func (c *Content) TasksOf(st *Step, mode string) []Task {
	sourced := c.StepTasks(st, mode)
	out := make([]Task, len(sourced))
	for i, t := range sourced {
		out[i] = t.Task
	}
	return out
}

// StepTask is one task of a step with where it comes from: the variant of
// the recipe of an objective the step completes, or the step itself (Recipe
// nil).
type StepTask struct {
	Task    Task
	Recipe  *Recipe
	Variant *RecipeVariant
}

// StepTasks is TasksOf with each task's recipe and variant, whose risks the
// executor applies (docs/10, E9).
func (c *Content) StepTasks(st *Step, mode string) []StepTask {
	var out []StepTask
	for _, o := range st.Objectives {
		if !o.Completing() {
			continue
		}
		if r, ok := c.Recipe(o.Key); ok {
			if v, ok := r.VariantFor(mode); ok {
				for _, t := range v.Tasks {
					out = append(out, StepTask{Task: t, Recipe: r, Variant: v})
				}
			}
		}
	}
	for _, t := range st.Tasks {
		out = append(out, StepTask{Task: t})
	}
	return out
}

var (
	recipeSchemaOnce     sync.Once
	recipeSchemaCompiled *jsonschema.Schema
	recipeSchemaErr      error
)

// RecipeSchema returns the compiled recipe schema.
func RecipeSchema() (*jsonschema.Schema, error) {
	recipeSchemaOnce.Do(func() {
		recipeSchemaCompiled, recipeSchemaErr = compileSchema(schema.RecipeSchemaURL, map[string][]byte{
			schema.GuideSchemaURL: schema.GuideSchema, schema.TaskSchemaURL: schema.TaskSchema, schema.RecipeSchemaURL: schema.RecipeSchema,
		})
	})
	return recipeSchemaCompiled, recipeSchemaErr
}

// compileSchema compiles the schema at url with the resources it references.
func compileSchema(url string, resources map[string][]byte) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	for u, raw := range resources {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("content: embedded schema %s is not valid JSON: %w", u, err)
		}
		if err := c.AddResource(u, doc); err != nil {
			return nil, fmt.Errorf("content: add schema resource: %w", err)
		}
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("content: compile %s: %w", url, err)
	}
	return sch, nil
}

// ValidateRecipeJSON checks one recipe file against the recipe schema.
func ValidateRecipeJSON(name string, data []byte) error {
	sch, err := RecipeSchema()
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return Errors{{File: name, Message: "invalid JSON: " + err.Error()}}
	}
	if err := sch.Validate(doc); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return schemaIssues(name, ve).errOrNil()
		}
		return Errors{{File: name, Message: err.Error()}}
	}
	return nil
}
