CREATE TABLE IF NOT EXISTS profiles (
 ggo_credit BIGINT NOT NULL DEFAULT 0,
 ggo_credit_migrated BOOLEAN NOT NULL DEFAULT 0,
 ggo_chip_total BIGINT NULL,
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  server_id BIGINT NOT NULL DEFAULT 1,
 account_id BIGINT NOT NULL DEFAULT 0,
 account_auth_revision BIGINT NOT NULL DEFAULT 0,
 uid VARBINARY(1024) NOT NULL,
  session LONGBLOB NOT NULL,
  player LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  game LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
 scene_scope VARCHAR(64) GENERATED ALWAYS AS (CASE
 WHEN game IS NULL OR player IS NULL OR JSON_VALID(game)=0 OR JSON_VALID(player)=0 OR ggo_credit<0 THEN NULL
 WHEN JSON_UNQUOTE(JSON_EXTRACT(game,'$.position.scene'))='town_union' THEN 'town_union'
 WHEN (JSON_TYPE(JSON_EXTRACT(game,'$.run'))='OBJECT' AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(game,'$.run.complete')),'false')<>'true') OR (JSON_TYPE(JSON_EXTRACT(game,'$.field.fight'))='OBJECT' AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(game,'$.field.fight.settled')),'false')<>'true') THEN ''
 WHEN JSON_UNQUOTE(JSON_EXTRACT(game,'$.position.scene')) IN ('town','town_main') THEN 'town_main'
 WHEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(game,'$.position_loading')),'false')='true' THEN ''
 WHEN JSON_UNQUOTE(JSON_EXTRACT(game,'$.position.scene')) IN ('field_001','field_002') THEN JSON_UNQUOTE(JSON_EXTRACT(game,'$.position.scene'))
 WHEN JSON_UNQUOTE(JSON_EXTRACT(game,'$.position.scene'))='field_003' AND JSON_TYPE(JSON_EXTRACT(game,'$.ggo_field'))='OBJECT' THEN 'field_003'
 ELSE '' END) STORED,
  last_sprout_login_at BIGINT NOT NULL DEFAULT 0,
  return_sprout_started_at BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NULL,
  password_salt LONGBLOB NULL,
  password_hash LONGBLOB NULL,
  UNIQUE KEY profiles_uid (uid),
  KEY idx_profiles_server_id (server_id)
) ENGINE=InnoDB;
