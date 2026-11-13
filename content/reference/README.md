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
| `items.json` | `item` | OSRS cache / RuneLite item id | generated from `data/wiki/items.json` (written by `tools/extract_wiki_items.py`) + curated item aliases and ambiguous names in `data/wiki/entity_aliases.json` |
| `skills.json` | `skill` | lower-case RuneLite `Skill` name | curated by hand |
| `unlocks.json` | `unlock` | slug | owner-confirmed proposals during extraction (`propose_entity`) |
| `locations.json` | `location` | slug | owner-confirmed proposals during extraction (`propose_entity`) |
| `varbits.json` | (vars) | varbit/varp id | generated from `data/samples/wiki-bucket/varbit_achievement_diary_all.json` + `data/samples/runelite/VarPlayer.java`: one `DIARY_<AREA>_<TIER>_COMPLETED` varbit per diary tier and the quest points varp |
| `npcs.json` | `npc` | slug of the wiki page title | generated from `data/wiki/npcs.json` (wiki buckets `infobox_npc` and `infobox_monster`, written by `tools/extract_wiki_world.py`) + id overrides in `data/wiki/world_ids.json` |
| `shops.json` | `shop` | slug of the wiki page title | generated from `data/wiki/shops.json` (buckets `infobox_shop` and `storeline`); stock items resolve against `items.json` |
| `objects.json` | `object` | slug of the wiki page title | generated from `data/wiki/objects.json` (bucket `infobox_scenery`) + id overrides in `data/wiki/world_ids.json` |
| `location_maps.json` | (map of `location`) | location id | generated from `data/wiki/location_maps.json` (the infobox `{{Map}}` of each location page, written by `tools/fetch_wiki_locations.py`): the map point and the area of each location |

`boss`, `goal` and `step` refs have no catalog: boss and goal ids are checked
syntactically, step ids against their guide. Objective keys (`quest:x@finished`,
`skill:agility>=70`, `item:960x120`, `qp>=107`; docs/09 section 3.2) are a ref
plus a target and are checked against these catalogs.

Regenerate the derived files (no network needed):

```sh
go test ./internal/core/entities/catalogbuild -update
```

The same test fails in CI when a generated file is stale. Each generated file
records the SHA-256 of its inputs in `source`.

Refreshing the world catalogs and the location maps needs the network once
(cached afterwards): `python3 tools/extract_wiki_world.py` and
`python3 tools/fetch_wiki_locations.py`, then the `-update` test above.

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
- `items.json` covers the items both guides cite plus the families they touch
  (potions, runes, axes, pickaxes, food, charged jewellery), about 850 ids, not
  the whole cache. Anything it does not list stays unresolved
  (`not_in_catalog`) and is reported, not guessed. Refreshing it is the only
  step that needs the network: `python3 tools/extract_wiki_items.py`, then the
  `-update` test below. The hand-curated subset it replaced is kept as a test
  fixture (`internal/core/entities/catalogbuild/testdata/items_curated.json`).
- Item families are addressed by `entities.FamilySlug` of the family name
  (`prayer_potion`, `antidote_plus_plus`) in `ItemRequirement.family`.
- Two ids with the same in-game name (Scrying orb full/empty, Ghostspeak
  amulet normal/enchanted) make the name ambiguous; the extraction picks one,
  or lists both in `any_of`.
- **World entities are wiki pages.** An `npc`, `shop` or `object` is one wiki
  page; `game_ids` lists the in-game ids of all its versions. A name several
  pages carry ("Banker", "Cook") resolves to the page titled with it when
  there is one, and is ambiguous otherwise; the page title itself ("Cook
  (Lumbridge)") always names one page. Two titles that slug alike fail the
  build until `data/wiki/world_ids.json` gives one of them an id; an id is
  never renamed by a later page. These kinds are not in the wiki-key index
  the importers resolve guide links with (`ByWikiKey`).
- **Shop stock never guesses.** A stock line enters `stock` only when its item
  (and currency, when not coins) resolves to one catalog item the way a guide
  name does, or its page holds exactly one item id; the rest is kept in the
  shop's `unresolved` with the reason.
- **Areas.** A location's area (where a `travel` task ends; docs/10 section 7)
  is the polygon, rectangle or square of its wiki infobox map, else the box of
  `entities.AreaRadius` (10) tiles around the map point. A location without a
  map has no area.

The importers (`ironman content migrate`, `ironman content import-bruhsailer`)
write the exact matches into each step's `entities` and loadout `item_id`, and
write everything ambiguous or unresolved to `entities.report.json` next to the
guide.
