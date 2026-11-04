// Package schema embeds the canonical JSON Schemas for guide content.
//
// content/schema/guide.schema.json is the source of truth for the content
// model and content/schema/annotations.schema.json for the annotation
// sidecars next to each guide (docs/09 section 4.2). The Go loader in
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
