// Package schema embeds the canonical JSON Schemas for guide content.
//
// content/schema/guide.schema.json is the source of truth for the content
// model, content/schema/annotations.schema.json for the annotation sidecars
// next to each guide (docs/09 section 4.2), task.schema.json for the typed
// tasks of the execution model and recipe.schema.json for the recipes under
// content/recipes (docs/10 sections 7 and 8). The Go loader in
// internal/core/content validates every file against them before decoding
// it into typed structs.
package schema

import _ "embed"

// GuideSchema is the raw JSON Schema (draft 2020-12) for content/guides/*/guide.json.
//
//go:embed guide.schema.json
var GuideSchema []byte

// GuideSchemaURL is the $id declared in the schema and the location used to compile it.
const GuideSchemaURL = "https://ironman-planner.local/schemas/guide.schema.json"

// AnnotationsSchema is the raw JSON Schema for content/guides/*/annotations/*.json.
//
//go:embed annotations.schema.json
var AnnotationsSchema []byte

// AnnotationsSchemaURL is the $id declared in the annotations schema.
const AnnotationsSchemaURL = "https://ironman-planner.local/schemas/annotations.schema.json"

// TaskSchema is the raw JSON Schema of one task (steps' tasks[] and recipes).
//
//go:embed task.schema.json
var TaskSchema []byte

// TaskSchemaURL is the $id declared in the task schema.
const TaskSchemaURL = "https://ironman-planner.local/schemas/task.schema.json"

// RecipeSchema is the raw JSON Schema for content/recipes/*.json.
//
//go:embed recipe.schema.json
var RecipeSchema []byte

// RecipeSchemaURL is the $id declared in the recipe schema.
const RecipeSchemaURL = "https://ironman-planner.local/schemas/recipe.schema.json"
