-- Switch the active route of an account (just account-guide / stack-account-guide).
-- psql variables: id, guide.
UPDATE accounts SET guide_id = :'guide', updated_at = now() WHERE id = :'id'
RETURNING id, rsn, guide_id, layer;
