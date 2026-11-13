// Package content holds the typed content model of a guide (Guide → Variant →
// Section → Segment → Step → Rule) and the loader that reads
// content/guides/*/guide.json,
// validates each file against the embedded JSON Schema
// (content/schema/guide.schema.json) and builds lookup indexes. The loader
// also composes the annotation sidecars of each guide and reads the recipes
// of content/recipes; tasks.go and recipes.go hold the typed tasks and
// recipes of the execution model and their verifier (docs/10 sections 7
// and 8).
//
// The package has no dependency on HTTP or the database; every other format
// of the binary (server, CLI, MCP) imports it.
package content
