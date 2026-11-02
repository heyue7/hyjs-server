CREATE TABLE IF NOT EXISTS profiles (
 ggo_credit INTEGER NOT NULL DEFAULT 0,
 ggo_credit_migrated INTEGER NOT NULL DEFAULT 0,
 ggo_chip_total INTEGER NULL,
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 server_id INTEGER NOT NULL DEFAULT 1,
 account_id INTEGER NOT NULL DEFAULT 0,
 account_auth_revision INTEGER NOT NULL DEFAULT 0,
 uid TEXT NOT NULL UNIQUE,
 session BLOB NOT NULL,
 player TEXT,
 player_level INTEGER GENERATED ALWAYS AS (CASE
 WHEN player IS NULL OR json_valid(player)=0 THEN NULL
 WHEN json_type(player,'$.level')='integer' AND json_extract(player,'$.level') BETWEEN 0 AND 2147483647 THEN json_extract(player,'$.level')
 ELSE NULL END) VIRTUAL,
 game TEXT,
 scene_scope VARCHAR(64) GENERATED ALWAYS AS (CASE
 WHEN game IS NULL OR player IS NULL OR json_valid(game)=0 OR json_valid(player)=0 OR ggo_credit<0 THEN NULL
 WHEN json_extract(game,'$.position.scene')='town_union' THEN 'town_union'
 WHEN (json_type(game,'$.run')='object' AND COALESCE(json_extract(game,'$.run.complete'),0)<>1) OR (json_type(game,'$.field.fight')='object' AND COALESCE(json_extract(game,'$.field.fight.settled'),0)<>1) THEN ''
 WHEN json_extract(game,'$.position.scene') IN ('town','town_main') THEN 'town_main'
 WHEN COALESCE(json_extract(game,'$.position_loading'),0)=1 THEN ''
 WHEN json_extract(game,'$.position.scene') IN ('field_001','field_002') THEN json_extract(game,'$.position.scene')
 WHEN json_extract(game,'$.position.scene')='field_003' AND json_type(game,'$.ggo_field')='object' THEN 'field_003'
 ELSE '' END) VIRTUAL,
 last_sprout_login_at INTEGER NOT NULL DEFAULT 0,
 return_sprout_started_at INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER,
 password_salt BLOB,
 password_hash BLOB
);
CREATE INDEX IF NOT EXISTS idx_profiles_server_level ON profiles (server_id, player_level, id);
