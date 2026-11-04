# Pinned BRUHsailer source

These three DOCX files are the offline source for the `bruhsailer-main` guide.
They come from the authoritative public mirror maintained at
<https://github.com/Osrsper/BRUHsailer>, whose README identifies the cached
Google Docs exports as the source of truth for the numbered guide steps.

## Pinned revision

- Repository: `https://github.com/Osrsper/BRUHsailer.git`
- Commit: `b697c0a63f120a9e35233503f13c1aeaf224bb2d`
- Commit time: `2026-11-04T01:51:37Z`
- Commit subject: `chore: auto-rebuild from Google Docs`
- Upstream source files: `source/Chapter1.docx`, `source/Chapter2.docx`,
  `source/Chapter3.docx`

The importer verifies these SHA-256 checksums before reading anything:

```text
9ae5795939a4d19fb97014e9b74f6b0adb536eb12ae8010511497b54c8e0e659  Chapter1.docx
35c373ba47f9d11cf0d29a8e87e26a1b17050eae96161d98a7757eb37ff112b4  Chapter2.docx
644e6160e9aa3c039a5795b5b381aa218e0e6351d063189ad1f718a2da023852  Chapter3.docx
```

The live Google Docs are intentionally not fetched during regeneration. To
update the pin, review a new upstream commit, replace all affected snapshots,
update the revision and checksums in both this file and the importer, then
review the generated diff.

## Attribution and licence caveat

BRUHsailer was written by **So Iron BRUH** in collaboration with
**ParasailerOSRS**. The upstream repository and documents do not declare an
explicit software or content licence. The snapshots and generated guide are
therefore retained only for personal, self-hosted use with attribution; do not
redistribute them publicly or use them commercially without permission from
the authors. The importer code is project-owned, but that does not change the
copyright status of the imported guide text.
