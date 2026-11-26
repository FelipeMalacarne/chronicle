-- Switch the active route of an account (just account-guide / stack-account-guide).
-- psql variables: id, guide (none: no route).
UPDATE accounts SET guide_id = NULLIF(:'guide', 'none'), updated_at = now() WHERE id = :'id'
RETURNING id, rsn, guide_id, layer;
