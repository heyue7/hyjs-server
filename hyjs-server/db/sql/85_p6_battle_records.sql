-- 原双方/配置/操作冻结，归属限制；不迁移旧奖励，不写当前钱包。
CREATE TABLE IF NOT EXISTS p6_battle_records (
 id VARCHAR(64) NOT NULL PRIMARY KEY, player_id BIGINT NOT NULL, source VARCHAR(32) NOT NULL,
 source_fight VARCHAR(128) NOT NULL, status VARCHAR(16) NOT NULL, seed BIGINT,
 configuration_hash VARCHAR(64) NOT NULL, engine_version VARCHAR(64), client_version VARCHAR(64),
 input LONGBLOB NOT NULL, operates LONGBLOB, outcome LONGBLOB,
 input_hash VARCHAR(64) NOT NULL, replay_hash VARCHAR(64), outcome_hash VARCHAR(64), client_result INT, server_result INT,
 authoritative BOOLEAN, created_at BIGINT, expires_at BIGINT, finished_at BIGINT,
 UNIQUE KEY p6_source(player_id,source,source_fight), KEY player_id(player_id),
 KEY p6_playback_alias(player_id,source_fight)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
