# BRUHsailer Ironman Guide

Guide id: `bruhsailer-main` · content version `2026.11.4` · 1 source
variant · 9 sections · 102 segments (99 explicit bank legs) · 226 steps.

This is an independent route. It is not merged into, layered over, or used to
replace `oziris-main`.

## Attribution and licence caveat

BRUHsailer was written by **So Iron BRUH** in collaboration with
**ParasailerOSRS**. The public source does not declare an explicit licence for
the guide text. This imported copy is for personal, self-hosted use only: keep
the attribution visible, and do not redistribute it publicly or use it
commercially without permission from the authors.

## Provenance

- Authoritative mirror: <https://github.com/Osrsper/BRUHsailer>
- Pinned commit: `b697c0a63f120a9e35233503f13c1aeaf224bb2d`
  (`2026-11-04T01:51:37Z`)
- Offline source snapshots and checksums:
  `data/sources/bruhsailer/README.md`
- Upstream source counts: 3 DOCX files, with 98 + 99 + 29 numbered steps.
- Imported result: 1 variant, 9 sections, 102 segments and 226 steps, in
  upstream order. Of the 99 bank legs, 84 preserve partial item requirements
  and 15 have an unknown loadout; all 3 non-bank sequences also remain
  explicitly unknown.

The upstream repository states that its cached `source/Chapter*.docx` Google
Docs exports are the source of truth for numbered steps. Regeneration never
fetches the live documents: the importer rejects any source file whose SHA-256
does not match the pinned revision.

## Mapping

- Explicit upstream section references (`1.1` through `3.2`) become project
  section ids such as `bruh-1.1`.
- Step ids are project-owned SHA-256 fingerprints of the section plus the
  step's ordered source text and metadata. They are deterministic and remain
  unchanged by unrelated insertions or reordering; they do not depend solely
  on array positions. The upstream positional reference is retained separately
  as `source.original_id` (for example, `CH1.1`).
- Ordered bullet paragraphs become the step text, with indentation retained.
- Hyperlinks become structured `links` in their original order.
- Items are split into the schema's `items` array while the exact upstream
  string is retained in a note. Exact GP, skills/quests, items and cumulative
  timing strings are retained as notes because schema v1 has no dedicated
  fields for all four upstream metadata types.
- Explicit bank/deposit/withdraw words start deterministic bank-leg segments.
  Their actions retain the exact matched text and source step as provenance;
  item metadata is copied into partial loadouts, while missing requirements
  are represented as `unknown` rather than as empty inventory/equipment.
- Segment ids are project-owned fingerprints of the section and first stable
  step id, so unrelated array insertions do not renumber them.
- The source describes one Ironman route. The import therefore exposes only
  the `ironman` default mode and does not invent HCIM/UIM/GIM or route variants;
  optional alternatives written inside a source step remain part of its text.

## Regeneration and validation

`guide.json` is generated; do not hand-edit it. Run:

```sh
go run ./cmd/ironman content import-bruhsailer
go test ./internal/importer/bruhsailer ./cmd/ironman
go run ./cmd/ironman content validate content/guides/bruhsailer-main/guide.json
```

The importer tests assert the pinned checksums, source count, per-chapter step
counts, section/step counts, metadata counts, byte-for-byte deterministic
regeneration, and canonical schema validation.
