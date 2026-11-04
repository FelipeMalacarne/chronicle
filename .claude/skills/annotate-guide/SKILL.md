---
name: annotate-guide
description: Extract objectives, item requirements, bank actions and locations from the steps of a guide (oziris-main, bruhsailer-main) into the verified annotation sidecars, through the ironman-content MCP server, with the owner watching and answering questions. Use when asked to annotate, extract or reprocess a guide section, or to fix stale annotations.
---

# annotate-guide

Skill version: **`annotate-guide@1`**. Pass it as `skill_version` on every `submit`. Bump the number
here whenever the taxonomy or procedure below changes.

You turn guide text into structured annotations (docs/09 section 6). The output is committed in
`content/guides/<id>/annotations/` and decides when a step is done for the owner's account, so a wrong
objective is worse than a missing one. Everything goes through the `ironman-content` MCP server
(`.mcp.json`, `ironman content mcp`). It resolves names, locates quotes, verifies every proposal with
the same checks as `ironman content validate`, and writes the files.

Hard rules:

- **Never edit** `guide.json`, the sidecars, `decisions.json`, `annotations.report.json` or
  `content/reference/*.json` yourself. Only the MCP tools write them.
- **Send names and quotes, never refs or spans.** The ref is always the one the resolver assigns. Use
  `id` only to pick among the candidates that `resolve_entity` or `submit` listed for that same name.
- **Nothing inferred enters without the owner.** `confidence: "high"` means the text says it.
  Anything you inferred is `"medium"` and needs either `confirmed_by: "owner"` (you asked in this
  session) or `decision: "<id>"` (a recorded general rule covers it). Otherwise leave it out and note it
  in `comment`.
- **Do not commit** unless the owner asks. Never commit an unconfirmed inference (CLAUDE.md rule 9).

## Before starting

1. Check that the `ironman-content` tools are available. If they are not, tell the owner to approve
   the project MCP server (it runs `go run ./cmd/ironman content mcp`) and stop.
2. Call `status` to see coverage, then `list_decisions` for the guide, and read every decision. Apply
   them without asking again, and cite the decision id in `decision` when you use one.
3. Agree on the section with the owner if they did not name one. Pilot order: Oziris `1.1`, then
   BRUHsailer `1.1`.

## Loop, per section

1. `next_batch(guide, section)`. Each step brings its quotable fields (`text`, `location`,
   `notes[i].text`, `items[i]`), the entity `mentions` the importers already resolved, the
   `rule_objectives` converted from its rules, and its `segment`. `previous` and `stale` mark a step
   whose text changed since it was annotated.
2. For each step, decide what it asks for (taxonomy below). Call `resolve_entity` for any name you are
   not sure resolves, before submitting.
3. Collect the questions for the batch and ask the owner (see "Asking the owner"). Then submit.
4. `submit(guide, section, skill_version, steps)`. Include every step of the batch, including steps
   with nothing to extract: submit them with no objectives and a short `comment`, so they count as
   reviewed.
5. Fix every rejected step and resubmit only those. A reason you cannot fix without guessing is a
   question for the owner. Do not drop it silently.
6. Repeat until `next_batch` returns no steps. Then call `report`, show the owner the summary
   (first-submit acceptance, rule agreement and its divergences, inferred entries, open rejections) and
   run `go run ./cmd/ironman content validate`.

Keep the owner informed with one line per batch: steps done, questions asked, rejections.

## Taxonomy (closed)

### Objectives

An objective is a game-state fact that the step makes true (`completes`), starts (`starts`), needs
beforehand (`requires`) or consumes on the way (`uses`).

| kind | name | target | roles |
|---|---|---|---|
| `quest` | quest name | `state`: `started` or `finished` | `starts` (only `started`), `completes` (only `finished`), `requires` |
| `skill` | skill name | `n`: level 1–99 | `completes`, `requires` |
| `diary` | area, plus `tier` | complete | `completes`, `requires` |
| `boss` | boss name | `n`: kill count | `completes`, `requires` |
| `item` | item name | `n`: quantity | `uses`, rarely `completes` (the step's goal is to obtain it) or `requires` |
| `unlock` | unlock name | done | `completes`, `requires` |
| `ca` | combat achievement task | done | `completes`, `requires` |
| `qp` | none | `n`: quest points | `requires` |

Each objective has a `quote`: the exact words that support it, from `field` (default `/text`). Case,
spacing and curly quotes may differ; the words may not.

Extract (positive examples from Oziris 1.1):

- "Talk to the duke to start Rune mysteries quest" → `quest` Rune Mysteries `started`, `starts`,
  high.
- "Finish the X marks the spot quest by talking to Veos" → `quest` X Marks the Spot `finished`,
  `completes`, high.
- "Light the 4 logs until 15 Firemaking" → `skill` Firemaking `n: 15`, `completes`, high.
- "Thieve men until 5 thieving" → `skill` Thieving `n: 5`, `completes`, high.
- "Buy a steel axe" → `item` Steel axe `n: 1`, `completes`, high (getting the axe is the step's goal).

Do not extract:

- Movement and talk with no state change: "Run to draynor village", "Walk to Falador",
  "Hug the fence and run west...". Submit these with no objectives.
- Advice: "Use Authenticator AND 2-step verification...", "use as emergency food if needed".
- Mid-quest actions as completions: "Dig up the clue north of bob's axes" is neither a start nor a
  finish of X Marks the Spot.
- A level the text only mentions in passing, or a goal stated for a later step.

Ask the owner (inferred, so `medium`):

- "Do Waterfall", "Cook's Assistant" without a verb of starting or finishing. "Do" usually means
  finish, but confirm the first time and record a decision for the rest.
- "Use every book and lamp on Herblore UNTIL 77 herb". The rule already gives `skill:herblore>=77
  completes` (derived, medium), but the step is an ongoing instruction, not something done now. Ask
  whether it completes the step.
- Requirements the text implies but does not state.

`rule_objectives` already apply to the step whether or not you repeat them. Repeat one when the text
supports it (that is the agreement metric). When the text contradicts a rule objective, do not repeat
it, and say why in `comment`: the report lists every divergence.

### Item requirements (`items`)

Items the step needs in the inventory (`slot: "inventory"`, the default) or worn (`"equipment"`). `name`
is the item as the text names it, without the quantity; `qty` is the interpreted number ("1k" =
1000). Exactly one way to resolve:

- the name alone, when it resolves to one item ("spade", "steel axe");
- `id`, one candidate of an ambiguous name, only when the text pins it down;
- `any_of`: item names in preference order, when several items serve and each must cover `qty` alone
  ("an axe" → best axe first, if a decision says so);
- `family`: every dose or charge counts together ("2 prayer potions" → `family: "prayer potion"`,
  `qty: 2`; "a games necklace"). Families come from `resolve_entity(kind: "item")`, which sets `family`
  when a name covers one.

`noted: true` (or "noted" in the name) means the stack must be noted.

Only steps whose `segment.starts_here` is true replace the segment's derived loadout. Check
`segment.derived_loadout` and keep what is right. On a step in the middle of a segment, items stay
requirements of that step.

Positive example: "Take out GP, Clue (x marks the spot), air talisman, spade, mind/air runes, bread and
shrimp from the bank" lists items to withdraw, plus a `withdraw` bank action. "GP" resolves to Coins and
"mind/air runes" is two items. The item catalog only holds what the guides cite, so check each name with
`resolve_entity`: one that is `not_in_catalog` (today "air talisman" and "shrimp") stays out of `items`
and goes in `comment`, which the report keeps for review.

### Bank actions (`bank_actions`)

Only what the text says, typed precisely, with `position` (`start`, `within`, `end`) in the step:

- "bank everything" → `deposit_all` (not `open_bank`)
- "Bank 7 logs for later", "Bank at least 4 ashes" → `deposit`
- "Take out ... from the bank" → `withdraw`
- a full inventory setup → `bank_setup`

### Location (`location`)

Only when the step says where it happens (`/location` or the text). Locations come from
`content/reference/locations.json`, which starts empty. When a location does not resolve, ask the owner
whether it deserves a catalog entry ("Draynor Village" yes; "West of Lumbridge" probably not). On yes,
call `propose_entity` with `confirmed_by: "owner"` (`wiki_key` is the wiki page title, and spellings the
guides use go in `aliases`), then resubmit. Unlocks work the same way (fairy rings, spellbooks, a
teleport, access to an area).

## Asking the owner

The owner watches the session and expects questions. Ask with **AskUserQuestion**, in **Portuguese
(PT-BR)**, keeping game terms (quest, skill and item names) in English:

- **When:** an objective is inferred; a name is ambiguous or does not resolve; the taxonomy does not fit
  (a new unlock or location, an uncertain role between `starts` and `completes`); the text contradicts the
  catalog or another step; the verifier rejects something and the fix is not obvious.
- **How:** group the questions by step, or by batch when they are of the same type (up to 4 per call).
  Show the exact quote, and offer concrete options with your recommendation first, marked
  "(Recomendado)".
- **Do not ask** what the text says explicitly, nor what a recorded decision already answers.

Record every answer:

- On the entries it decided, as `confirmed_by: "owner"`.
- When the answer is a general rule ("'Axe' sem qualificador = o melhor axe disponível"), also call
  `record_decision` (id a short slug, question and answer in PT-BR, `scope` saying where it holds, the
  `step_ids` where it came up). From then on, use `decision: "<id>"` on the entries it covers, without
  asking.
- When the owner leaves something open, it stays out of the sidecar. Put it in `comment`.

## Rejections you will see

- `quote ... is not in /text`: quote the words exactly, or point `field` at the note or item that
  holds them.
- `is ambiguous ... choose an id among ...`: pick one only if the text says which. Otherwise use
  `any_of` or `family`, or ask the owner.
- `does not resolve`: check spelling with `resolve_entity`. Unlocks and locations may need
  `propose_entity`. Items outside the catalog stay out; mention them in `comment`.
- `needs confirmed_by owner`: you sent an inference. Ask first.
- `order: ...`: the objective contradicts the route (a quest started after it is finished, something
  completed twice, a requirement the route only meets later). Usually the role or state is wrong.
  Otherwise ask the owner.
