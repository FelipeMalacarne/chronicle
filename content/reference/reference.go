// Package reference embeds the versioned entity catalogs under
// content/reference (quests, items, skills, diaries, combat achievements).
//
// quests.json, diaries.json and combat_achievements.json are generated
// offline from data/wiki and data/samples by internal/core/entities/catalogbuild
// (go test ./internal/core/entities/catalogbuild -update); items.json and
// skills.json are curated by hand. internal/core/entities builds its
// Registry from these files.
package reference

import "embed"

// FS holds every catalog file.
//
//go:embed *.json
var FS embed.FS
