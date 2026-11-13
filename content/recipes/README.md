# Recipes

A recipe says how to fulfill one objective key (docs/09 section 3.2), whatever
the route: the typed tasks the executor runs, per account mode, with their
risks (docs/10 section 8). A guide step that completes a key with a recipe
inherits the recipe's tasks (`content.Content.TasksOf`); the step's own tasks
(`tasks[]` in its annotation sidecar) are only what is the route's ("fire
strike gnome guards until 43").

This directory is empty on purpose. Recipes and step tasks come from guide
extraction with the owner watching, or from recording a demonstration in
game (docs/10 phase 2), never typed by hand from memory: CLAUDE.md rule 9.
Phase 1 delivered the format, the loader and the verifier, proved with the
fixtures in `internal/core/content/testdata/execution`.

## Format

One file per objective key, named after the key: every run of characters
outside `[a-z0-9_]` becomes `-` (`content.RecipeFileName`).

| Key | File |
|---|---|
| `quest:cooks_assistant@finished` | `quest-cooks_assistant-finished.json` |
| `skill:attack>=10` | `skill-attack-10.json` |
| `item:1351x1` | `item-1351x1.json` |

```json
{
  "schema_version": 1,
  "kind": "recipe",
  "objective": "skill:attack>=10",
  "requires": ["skill:hitpoints>=10"],
  "variants": [
    {
      "id": "main",
      "modes": ["ironman", "uim", "gim"],
      "risks": [{ "kind": "combat", "npc": "npc:goblin" }],
      "tasks": [
        { "type": "bank", "items": [{ "name": "Bronze sword", "item_id": 1277, "qty": 1 }] },
        { "type": "travel", "location": "location:lumbridge" },
        { "type": "train", "skill": "skill:attack", "npc": "npc:goblin", "until": { "objective": "skill:attack>=10" } }
      ]
    },
    {
      "id": "hcim",
      "modes": ["hcim"],
      "risks": [{ "kind": "combat", "npc": "npc:chicken", "location": "location:lumbridge" }],
      "tasks": [
        { "type": "travel", "location": "location:lumbridge" },
        { "type": "train", "skill": "skill:attack", "npc": "npc:chicken", "until": { "objective": "skill:attack>=10" } }
      ]
    }
  ],
  "source": { "method": "manual", "confidence": "high", "confirmed_by": "owner" }
}
```

- `objective`: the key the recipe fulfills. `requires`: preconditions beyond
  the catalog's (a quest's own requirements already come from `quests.json`).
- `variants`: one per group of account modes (`ironman`, `hcim`, `uim`,
  `gim`); a mode is in at most one variant.
- `risks`: from a closed list. `combat`, `wilderness` and `hazard` can kill;
  `item_loss` cannot. In HCIM nothing with a death risk runs unsupervised
  (docs/10, E9; `RecipeVariant.Unsupervised`). The executor (phase 3) applies
  it; the recipe records it.
- `tasks`: the schema is `content/schema/task.schema.json`, the same one as
  the steps' `tasks[]`:

| Type | Parameters | Ends (`until`) |
|---|---|---|
| `travel` | `location`, `via?` (unlock or teleport item) | `at_location`, the default: inside the location's area (`location_maps.json`) |
| `bank` | `items` (required in a recipe; in a step, the segment's loadout) | `loadout_ready`, the default |
| `buy` | `shop`, `item`, `qty` | `item:<id>x<qty>`, the default |
| `talk` | `npc`, `options?` (the game's dialogue text), `location?` | an objective key or `goal_met` |
| `interact` | `object`, `action` (the game's menu text), `location?` | an objective key or `goal_met` |
| `train` | `skill`, `npc?` or `object?`, `location?`, `plugin?` (the Microbot plugin class the executor delegates to; without it the owner trains), `config?` (`[{group, key, value}]`, its settings, set as they are) | `skill:<skill>>=N` of the same skill |
| `manual` | `note` (required in a recipe: what to tell the owner) | an objective key or `goal_met` (`owner_mark` only in a step) |

`goal_met` is the recipe's objective here (in a step, the objectives the step
completes). `quest` tasks come later (E5).

## What `chronicle content validate` checks

The schema, then `content.CheckRecipe`:

- the file is named after its objective, and no two files fulfill one key;
- the objective and every requirement are keys the catalogs know;
- variant ids are unique and a mode is in one variant only;
- every risk is of the closed list, with known `npc` and `location` refs;
- every task passes `Task.Check`: a type of the executor's list, only the
  parameters of its type, every ref a catalog entry of the right kind (a shop
  that sells the item per its wiki stock, a travel destination with an
  area), and a termination the engine can evaluate;
- the last task of each variant ends on the recipe's objective;
- an inference (`confidence` other than `high`) was confirmed by the owner.
  Recipes belong to no guide, so they cite no decision.
