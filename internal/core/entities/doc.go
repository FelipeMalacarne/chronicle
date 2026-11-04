// Package entities is the canonical domain entity registry: typed refs for
// quests, items, skills, diaries, bosses, combat achievements, unlocks,
// locations, goals and steps; objective keys (a ref plus a target state);
// the versioned catalogs embedded from content/reference; and a resolver
// that maps source text to refs only through exact, deterministic mappings
// (canonical name, catalog alias, article-less spelling, plural, item id,
// wiki page key). Anything else is reported as ambiguous or unresolved with
// candidates, never guessed.
//
// Like internal/core/content it is pure: no database, HTTP or network.
package entities
