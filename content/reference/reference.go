// Package reference embeds the versioned entity catalogs under
// content/reference (quests, items, skills, diaries, combat achievements,
// unlocks, locations, reference vars).
//
// quests.json, items.json, diaries.json, combat_achievements.json and
// varbits.json are generated offline from data/wiki and data/samples by
// internal/core/entities/catalogbuild (go test
// ./internal/core/entities/catalogbuild -update); skills.json is curated by
// hand; unlocks.json and locations.json grow only through owner-confirmed
// proposals during guide extraction. internal/core/entities builds its
// Registry from these files.
package reference

import "embed"

// FS holds every catalog file.
//
//go:embed *.json
var FS embed.FS
