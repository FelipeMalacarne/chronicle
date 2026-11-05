# OSRS Ironman Efficiency Guide (OzirisLoL)

Guide id: `oziris-main` · content version `2026.11.0` · 1 source variant · 7 sections · 31 segments (24 explicit bank legs) · 578 steps (209 with automatic rules, 369 manual).

## Attribution

- Author: **OzirisLoL** (the original "OSRS Ironman Efficiency Guide", first published as a public Google Doc).
- Republished, reorganized and extended ("enhanced" steps, update notes) by **ironman.guide** (https://ironman.guide/guide).
- No explicit license is declared by either layer. This copy is for personal, self-hosted use only:
  keep this attribution, show it in the UI, do not redistribute publicly or use commercially.

## Provenance

- Extracted from https://ironman.guide/guide on 2026-11-03 (`tools/extract_page.py`, raw pages under `data/guide/`).
- Normalised by `tools/normalize_guide.py` into `data/guide/normalized/guide.json` (schema v0.1).
- Migrated to the canonical schema (`content/schema/guide.schema.json`) by
  `ironman content migrate` (package `internal/core/content/migrate`).
  Field mapping: docs/05-conteudo-e-normalizacao.md, "Mapeamento para o schema canônico (Go)".
- Route corrected by 2 owner patches from `data/guide/route-patches.json` (dropped, moved and added steps;
  added steps have source origin `own`). See docs/05-conteudo-e-normalizacao.md, section 12.
- Explicit bank/deposit/withdraw words deterministically start a bank-leg
  segment. The checked-in result contains 5 partial loadouts and 26 explicitly
  unknown loadouts; absent requirements are never represented as empty arrays.
- Bank actions and structured loadouts retain source step ids, confidence and
  derivation evidence. Segment ids are fingerprints of the section and first
  stable step id, independent of array positions.

## Editing

`guide.json` is generated. Do not hand-edit it while the normalizer is still the source of truth;
fix the extraction/normalisation (or the route patches) and re-run:

    go run ./cmd/ironman content migrate
    go run ./cmd/ironman content validate

Step ids are stable and inherited from ironman.guide; never reuse one.
