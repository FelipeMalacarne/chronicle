-- Ironman Planner — esquema do banco (SQLite, portável para Postgres via Drizzle).
-- Convenções: ids TEXT (ulid), timestamps TEXT ISO-8601 UTC, JSON em TEXT (SQLite) / jsonb (Postgres).
-- PRAGMAs aplicados no boot: journal_mode=WAL, foreign_keys=ON, busy_timeout=5000, synchronous=NORMAL.

------------------------------------------------------------------------------
-- 1. Identidade e acesso
------------------------------------------------------------------------------
CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,                -- argon2id
  role          TEXT NOT NULL DEFAULT 'owner' CHECK (role IN ('owner','member')),
  created_at    TEXT NOT NULL,
  last_login_at TEXT
);

CREATE TABLE sessions (
  id          TEXT PRIMARY KEY,               -- hash sha256 do cookie
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  user_agent  TEXT
);
CREATE INDEX sessions_user_idx ON sessions(user_id);

-- Tokens de API para scripts/CLI (não para o plugin).
CREATE TABLE api_tokens (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL UNIQUE,
  label       TEXT,
  created_at  TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at  TEXT
);

------------------------------------------------------------------------------
-- 2. Contas de jogo
------------------------------------------------------------------------------
CREATE TABLE accounts (
  id             TEXT PRIMARY KEY,
  user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  rsn            TEXT NOT NULL,                 -- nome atual, como aparece no hiscores
  rsn_normalized TEXT NOT NULL,                 -- lower, espaços -> '_' (chave de lookup)
  mode           TEXT NOT NULL CHECK (mode IN ('ironman','hcim','uim','gim')),
  guide_id       TEXT NOT NULL,                 -- guia ativo (content)
  layer          TEXT NOT NULL DEFAULT 'enhanced' CHECK (layer IN ('classic','enhanced')),
  wom_player_id  INTEGER,                       -- id no Wise Old Man, se vinculado
  name_history   TEXT NOT NULL DEFAULT '[]',    -- JSON [{rsn, from, to, detected_by}]
  active         INTEGER NOT NULL DEFAULT 1,
  created_at     TEXT NOT NULL,
  UNIQUE (user_id, rsn_normalized)
);

-- Configurações: account_id NULL = global.
CREATE TABLE settings (
  account_id TEXT REFERENCES accounts(id) ON DELETE CASCADE,
  key        TEXT NOT NULL,
  value      TEXT NOT NULL,                     -- JSON
  updated_at TEXT NOT NULL,
  PRIMARY KEY (account_id, key)
);

-- Tokens do plugin RuneLite: um por instalação/cliente, escopo = uma conta.
CREATE TABLE plugin_ingest_tokens (
  id               TEXT PRIMARY KEY,
  account_id       TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  token_hash       TEXT NOT NULL UNIQUE,        -- sha256 do token 'ipl_...' (mostrado uma vez)
  label            TEXT,                        -- 'PC casa', 'notebook'
  scopes           TEXT NOT NULL DEFAULT '["vars","skills","collection_log"]', -- JSON; 'bank','inventory','equipment' são opt-in
  manifest_version INTEGER NOT NULL DEFAULT 0,  -- última versão do manifest entregue a este cliente
  last_seq         INTEGER NOT NULL DEFAULT 0,  -- último client_seq aceito (idempotência/ordem)
  last_seen_at     TEXT,
  last_ip          TEXT,
  created_at       TEXT NOT NULL,
  revoked_at       TEXT
);
CREATE INDEX pit_account_idx ON plugin_ingest_tokens(account_id);

------------------------------------------------------------------------------
-- 3. Sync e snapshots (append-only)
------------------------------------------------------------------------------
CREATE TABLE sync_runs (
  id              TEXT PRIMARY KEY,
  account_id      TEXT REFERENCES accounts(id) ON DELETE CASCADE, -- NULL para jobs globais (wiki bucket)
  source          TEXT NOT NULL CHECK (source IN ('hiscores','wom','wikisync','plugin','wiki_bucket','import')),
  trigger         TEXT NOT NULL CHECK (trigger IN ('scheduled','manual','push','boot')),
  status          TEXT NOT NULL CHECK (status IN ('queued','running','ok','unchanged','skipped','rate_limited','error')),
  started_at      TEXT,
  finished_at     TEXT,
  http_status     INTEGER,
  error           TEXT,
  snapshot_id     TEXT,                          -- preenchido quando gerou snapshot novo
  steps_changed   INTEGER NOT NULL DEFAULT 0,    -- quantos steps mudaram de status na reavaliação
  next_allowed_at TEXT,                          -- backoff calculado após 429/5xx
  created_at      TEXT NOT NULL
);
CREATE INDEX sync_runs_account_source_idx ON sync_runs(account_id, source, created_at DESC);

CREATE TABLE snapshots (
  id             TEXT PRIMARY KEY,
  account_id     TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  source         TEXT NOT NULL CHECK (source IN ('hiscores','wom','wikisync','plugin','import')),
  kind           TEXT NOT NULL DEFAULT 'full' CHECK (kind IN ('full','delta')),
  fetched_at     TEXT NOT NULL,
  raw            TEXT NOT NULL,                  -- JSON bruto como veio (reprocessável)
  raw_hash       TEXT NOT NULL,                  -- sha256(raw) para dedupe
  adapter_version INTEGER NOT NULL,              -- versão do normalizador usado
  client_seq     INTEGER,                        -- só plugin
  UNIQUE (account_id, source, raw_hash)
);
CREATE INDEX snapshots_account_time_idx ON snapshots(account_id, fetched_at DESC);

-- Tabelas normalizadas por snapshot (histórico; alimentam gráficos e diffs).
CREATE TABLE snapshot_skills (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  skill       TEXT NOT NULL,
  level       INTEGER NOT NULL,
  xp          INTEGER,
  rank        INTEGER,
  boosted     INTEGER,                           -- só plugin
  PRIMARY KEY (snapshot_id, skill)
);

CREATE TABLE snapshot_activities (                -- bosses, clues, minigames, raids
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  activity    TEXT NOT NULL,                     -- nome canônico do hiscores
  score       INTEGER NOT NULL,                  -- -1 = unranked
  rank        INTEGER,
  PRIMARY KEY (snapshot_id, activity)
);

CREATE TABLE snapshot_quests (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  quest       TEXT NOT NULL,                     -- nome canônico wiki
  state       TEXT NOT NULL CHECK (state IN ('NOT_STARTED','IN_PROGRESS','FINISHED')),
  PRIMARY KEY (snapshot_id, quest)
);

CREATE TABLE snapshot_diary_tasks (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  area        TEXT NOT NULL,
  tier        TEXT NOT NULL CHECK (tier IN ('easy','medium','hard','elite')),
  task_index  INTEGER NOT NULL,
  done        INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, area, tier, task_index)
);

CREATE TABLE snapshot_combat_achievements (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  ca_id       INTEGER NOT NULL,
  done        INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, ca_id)
);

CREATE TABLE snapshot_collection_log (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  item_id     INTEGER NOT NULL,
  qty         INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (snapshot_id, item_id)
);

CREATE TABLE snapshot_vars (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK (kind IN ('varbit','varp')),
  var_id      INTEGER NOT NULL,
  value       INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, kind, var_id)
);

CREATE TABLE snapshot_items (                      -- banco/inventário/equipamento (opt-in)
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  container   TEXT NOT NULL CHECK (container IN ('bank','inventory','equipment','poh')),
  item_id     INTEGER NOT NULL,
  qty         INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, container, item_id)
);

------------------------------------------------------------------------------
-- 4. Estado atual mesclado (entrada do rule engine)
------------------------------------------------------------------------------
-- Uma linha por fato conhecido. Fontes diferentes cobrem facetas diferentes;
-- o merge é "última observação vence" dentro da faceta, com prioridade por fonte
-- em caso de empate de timestamp (plugin > wikisync > hiscores > wom).
CREATE TABLE player_state (
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  facet       TEXT NOT NULL CHECK (facet IN ('skill','activity','quest','diary_task','combat_achievement','collection_log','varbit','varp','item')),
  key         TEXT NOT NULL,                     -- 'agility' | 'Zulrah' | 'Lost City' | 'Ardougne:easy:3' | '12345' | 'bank:4151'
  value       TEXT NOT NULL,                     -- JSON: {"level":56,"xp":...} | {"score":120} | {"state":"FINISHED"} | {"done":true} | {"qty":1} | {"value":1}
  source      TEXT NOT NULL,
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id),
  observed_at TEXT NOT NULL,
  PRIMARY KEY (account_id, facet, key)
);
CREATE INDEX player_state_observed_idx ON player_state(account_id, facet, observed_at);

------------------------------------------------------------------------------
-- 5. Conteúdo carregado (índice; a fonte da verdade é content/ no git)
------------------------------------------------------------------------------
CREATE TABLE content_versions (
  id            TEXT PRIMARY KEY,
  content_hash  TEXT NOT NULL UNIQUE,            -- sha256 do bundle canônico
  git_sha       TEXT,
  guide_version TEXT,
  loaded_at     TEXT NOT NULL,
  step_count    INTEGER NOT NULL
);

CREATE TABLE content_steps (                       -- um por step do guia carregado atualmente
  step_id       TEXT PRIMARY KEY,
  guide_id      TEXT NOT NULL,
  section_id    TEXT NOT NULL,
  position      INTEGER NOT NULL,                -- ordem global no guia
  rule_hash     TEXT NOT NULL,                   -- sha256(complete_when canônico); muda => reavaliar
  is_manual     INTEGER NOT NULL,
  layers        TEXT NOT NULL,                   -- JSON
  modes         TEXT NOT NULL,                   -- JSON
  content_version_id TEXT NOT NULL REFERENCES content_versions(id)
);
CREATE INDEX content_steps_guide_idx ON content_steps(guide_id, position);

-- Remapeamento de IDs declarado em content/migrations/*.json (replaces).
CREATE TABLE step_id_aliases (
  old_step_id TEXT PRIMARY KEY,
  new_step_id TEXT NOT NULL,
  applied_at  TEXT
);

------------------------------------------------------------------------------
-- 6. Progresso por conta
------------------------------------------------------------------------------
CREATE TABLE step_progress (
  account_id           TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  step_id              TEXT NOT NULL,            -- sem FK: progresso sobrevive a steps removidos do conteúdo
  status               TEXT NOT NULL CHECK (status IN ('pending','auto_done','manual_done','skipped')),
  completed_at         TEXT,
  evidence_snapshot_id TEXT REFERENCES snapshots(id),
  evidence             TEXT,                     -- JSON: [{rule, observed, satisfied, source, observed_at}]
  rule_hash            TEXT,                     -- hash da regra que produziu auto_done
  manual_locked        INTEGER NOT NULL DEFAULT 0, -- 1 = usuário fixou status; engine não mexe
  manual_at            TEXT,
  regressed_at         TEXT,                     -- engine viu a regra deixar de valer depois de auto_done
  user_note            TEXT,
  updated_at           TEXT NOT NULL,
  PRIMARY KEY (account_id, step_id)
);
CREATE INDEX step_progress_status_idx ON step_progress(account_id, status);

-- Histórico de transições (alimenta "o que destravou/concluiu hoje").
CREATE TABLE step_progress_events (
  id           TEXT PRIMARY KEY,
  account_id   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  step_id      TEXT NOT NULL,
  from_status  TEXT,
  to_status    TEXT NOT NULL,
  cause        TEXT NOT NULL CHECK (cause IN ('engine','user','migration','import')),
  snapshot_id  TEXT,
  created_at   TEXT NOT NULL
);
CREATE INDEX spe_account_time_idx ON step_progress_events(account_id, created_at DESC);

CREATE TABLE goals (
  id          TEXT PRIMARY KEY,
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  ref         TEXT NOT NULL,                     -- 'quest:Dragon Slayer II' | 'item:Barrows gloves' | 'skill:agility:70' | 'step:1.5.21'
  label       TEXT NOT NULL,
  priority    INTEGER NOT NULL DEFAULT 100,
  status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','achieved','abandoned')),
  plan_cache  TEXT,                              -- JSON: último plano topológico calculado
  created_at  TEXT NOT NULL,
  achieved_at TEXT
);
CREATE INDEX goals_account_idx ON goals(account_id, status, priority);

------------------------------------------------------------------------------
-- 7. Caches externos
------------------------------------------------------------------------------
CREATE TABLE wiki_cache (                          -- Bucket API e outros dados de referência
  kind       TEXT NOT NULL CHECK (kind IN ('quest','diary','item','boss','varbit_map','ca')),
  key        TEXT NOT NULL,                       -- nome canônico
  value      TEXT NOT NULL,                       -- JSON normalizado pelo adapter
  fetched_at TEXT NOT NULL,
  etag       TEXT,
  PRIMARY KEY (kind, key)
);

CREATE TABLE graph_cache (
  id                 TEXT PRIMARY KEY,
  content_version_id TEXT NOT NULL REFERENCES content_versions(id),
  wiki_fingerprint   TEXT NOT NULL,              -- hash das linhas de wiki_cache usadas
  built_at           TEXT NOT NULL,
  nodes_json         TEXT NOT NULL,
  edges_json         TEXT NOT NULL
);

------------------------------------------------------------------------------
-- 8. Views de conveniência
------------------------------------------------------------------------------
CREATE VIEW v_current_skills AS
  SELECT account_id, key AS skill,
         json_extract(value,'$.level') AS level,
         json_extract(value,'$.xp')    AS xp,
         source, observed_at
  FROM player_state WHERE facet = 'skill';

CREATE VIEW v_current_quests AS
  SELECT account_id, key AS quest, json_extract(value,'$.state') AS state, source, observed_at
  FROM player_state WHERE facet = 'quest';

CREATE VIEW v_step_board AS                        -- o que a UI lista
  SELECT cs.guide_id, cs.section_id, cs.position, cs.step_id, cs.is_manual,
         sp.account_id, COALESCE(sp.status,'pending') AS status,
         sp.completed_at, sp.manual_locked, sp.regressed_at
  FROM content_steps cs
  LEFT JOIN step_progress sp ON sp.step_id = cs.step_id;
