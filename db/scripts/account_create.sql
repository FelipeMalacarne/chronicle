-- Dev account until the account screen exists (just account-create / stack-account-create).
-- psql variables: rsn, mode, guide, layer. The id is acc_<rsn normalized>.
INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev') ON CONFLICT DO NOTHING;
INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
VALUES ('acc_' || replace(lower(trim(:'rsn')), ' ', '_'), 'dev', trim(:'rsn'),
        replace(lower(trim(:'rsn')), ' ', '_'), :'mode', :'guide', :'layer')
RETURNING id, rsn, mode, guide_id, layer;
