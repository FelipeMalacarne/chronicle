-- Persistence helpers for the loaded content index. Local ids are always
-- paired with guide_id; callers must never synthesize a global step id.

-- name: UpsertContentVersion :one
-- The same bundle loaded again keeps its id and only refreshes loaded_at.
INSERT INTO content_versions (id, content_hash, git_sha, summary)
VALUES ($1, $2, $3, $4)
ON CONFLICT (content_hash) DO UPDATE SET
  git_sha   = EXCLUDED.git_sha,
  summary   = EXCLUDED.summary,
  loaded_at = now()
RETURNING *;

-- name: DeleteContentStepsByGuide :execrows
DELETE FROM content_steps WHERE guide_id = $1;

-- name: InsertContentSteps :copyfrom
INSERT INTO content_steps (guide_id, step_id, section_id, position, text_hash, content_version_id)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListContentStepsByGuide :many
SELECT * FROM content_steps
WHERE guide_id = $1
ORDER BY position, step_id;

-- name: UpsertStepIDAlias :one
-- Declared by a step's `replaces`. Declaring the same mapping again keeps
-- applied_at; pointing the old id somewhere else applies it again.
INSERT INTO step_id_aliases (guide_id, old_step_id, new_step_id)
VALUES ($1, $2, $3)
ON CONFLICT (guide_id, old_step_id) DO UPDATE SET
  new_step_id = EXCLUDED.new_step_id,
  applied_at  = CASE WHEN step_id_aliases.new_step_id = EXCLUDED.new_step_id
                     THEN step_id_aliases.applied_at END
RETURNING *;

-- name: ResolveStepIDAlias :one
SELECT * FROM step_id_aliases
WHERE guide_id = $1 AND old_step_id = $2;

-- name: ListPendingStepIDAliases :many
SELECT * FROM step_id_aliases
WHERE guide_id = $1 AND applied_at IS NULL
ORDER BY old_step_id;

-- name: MarkStepIDAliasApplied :exec
UPDATE step_id_aliases SET applied_at = now()
WHERE guide_id = $1 AND old_step_id = $2;

-- name: UpsertGuide :exec
-- Every guide the server loads; progress rows reference it.
INSERT INTO guides (id, title, last_loaded_at) VALUES ($1, $2, now())
ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, last_loaded_at = now();
