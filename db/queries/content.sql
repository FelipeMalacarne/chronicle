-- Persistence helpers for the loaded content index. Local ids are always
-- paired with guide_id; callers must never synthesize a global step id.

-- name: InsertContentVersion :one
INSERT INTO content_versions (id, content_hash, git_sha, guide_version, step_count)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpsertContentStep :one
INSERT INTO content_steps (
  guide_id, step_id, section_id, position, rule_hash, is_manual, layers, modes, content_version_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (guide_id, step_id) DO UPDATE SET
  section_id         = EXCLUDED.section_id,
  position           = EXCLUDED.position,
  rule_hash          = EXCLUDED.rule_hash,
  is_manual          = EXCLUDED.is_manual,
  layers             = EXCLUDED.layers,
  modes              = EXCLUDED.modes,
  content_version_id = EXCLUDED.content_version_id
RETURNING *;

-- name: DeleteContentStepsByGuide :execrows
DELETE FROM content_steps WHERE guide_id = $1;

-- name: ListContentStepsByGuide :many
SELECT * FROM content_steps
WHERE guide_id = $1
ORDER BY position, step_id;

-- name: UpsertStepIDAlias :one
INSERT INTO step_id_aliases (guide_id, old_step_id, new_step_id, applied_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (guide_id, old_step_id) DO UPDATE SET
  new_step_id = EXCLUDED.new_step_id,
  applied_at  = EXCLUDED.applied_at
RETURNING *;

-- name: ResolveStepIDAlias :one
SELECT * FROM step_id_aliases
WHERE guide_id = $1 AND old_step_id = $2;
