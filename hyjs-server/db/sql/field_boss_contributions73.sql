CREATE TABLE IF NOT EXISTS field_boss_contributions73 (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 server_id BIGINT NOT NULL,
 spawn_uuid VARBINARY(128) NOT NULL,
 player_id BIGINT NOT NULL,
 damage DOUBLE NOT NULL,
 rules LONGBLOB NOT NULL,
 sprouts BIGINT NOT NULL,
 daily_limit BIGINT NOT NULL,
 reward TEXT NOT NULL,
 reward_day CHAR(10) NOT NULL,
 reward_rank BIGINT NOT NULL,
 frozen BOOLEAN NOT NULL,
 claimed BOOLEAN NOT NULL,
 UNIQUE KEY uidx_boss73_member(server_id,spawn_uuid,player_id),
 KEY idx_boss73_pending(server_id,player_id,frozen,claimed),
 KEY idx_boss73_day(server_id,player_id,reward_day,claimed)
);
