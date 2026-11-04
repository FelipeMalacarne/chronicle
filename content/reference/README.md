# Entity catalogs

Versioned, offline catalogs behind the typed entity refs
(`internal/core/entities`). Guides store identities such as
`quest:the_restless_ghost` or `item:961`; display names and wiki URLs are
derived from these files, never stored in the guide.

| File | Kind | Id | Origin |
|---|---|---|---|
| `quests.json` | `quest` | slug of the canonical wiki name | generated from `data/wiki/quests.json` + curated aliases in `data/wiki/entity_aliases.json` |
| `diaries.json` | `diary` | `<area slug>:<tier>` | generated from `data/wiki/diary_tasks.json` |
| `combat_achievements.json` | `ca` | game task id | generated from `data/samples/wiki-bucket/ca_all.json` |
| `items.json` | `item` | OSRS cache / RuneLite item id | curated by hand (subset) |
| `skills.json` | `skill` | lower-case RuneLite `Skill` name | curated by hand |

`boss`, `goal` and `step` refs have no catalog: boss and goal ids are checked
syntactically, step ids against their guide.

Regenerate the derived files (no network needed):

```sh
go test ./internal/core/entities/catalogbuild -update
```

The same test fails in CI when a generated file is stale. Each generated file
records the SHA-256 of its inputs in `source`.

## Rules

- **Wiki keys, not URLs.** Each entry has a `wiki_key` (page title with
  spaces); article URLs are derived from it. Links found in guides are reduced
  to a key before lookup.
- **Noted forms are different ids.** `items.json` lists unnoted ids; `noted_id`
  is the id the plugin reports for a noted stack. `item:961` (noted plank) is a
  valid ref distinct from `item:960`. Comparing them as equal requires an
  explicit `CompareIgnoreNoted`.
- **Variants are explicit.** Doses and charges are separate ids that share a
  `family`. A family name alone ("prayer potions") is `variant_unspecified`,
  never defaulted to one dose. `ambiguous` lists names that must never resolve
  (e.g. "pickaxe", "Amulet of glory").
- **Curated aliases carry a note.** Quest aliases from WikiSync/RuneLite are
  copied; anything else lives in `data/wiki/entity_aliases.json` with a reason.
  `text_ambiguous` names (Tears of Guthix, RFD) do not resolve from free text
  unless the text itself says "quest".
- `items.json` is a hand-curated subset, so anything it does not list stays
  unresolved (`not_in_catalog`) and is reported, not guessed.

The importers (`ironman content migrate`, `ironman content import-bruhsailer`)
write the exact matches into each step's `entities` and loadout `item_id`, and
write everything ambiguous or unresolved to `entities.report.json` next to the
guide.
