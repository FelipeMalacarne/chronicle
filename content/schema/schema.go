// Package schema embeds the canonical JSON Schema for guide content.
//
// content/schema/guide.schema.json is the source of truth for the content
// model (docs/arquitetura/guide.schema.json is kept only as a reference copy).
// The Go loader in internal/core/content validates every guide file against
// this schema before decoding it into typed structs.
package schema

import _ "embed"

// GuideSchema is the raw JSON Schema (draft 2020-12) for content/guides/*/guide.json.
//
//go:embed guide.schema.json
var GuideSchema []byte

// GuideSchemaURL is the $id declared in the schema and the location used to compile it.
const GuideSchemaURL = "https://ironman-planner.local/schemas/guide.schema.json"
