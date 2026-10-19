CREATE TABLE IF NOT EXISTS admin_character_activity_instances (
 player_id BIGINT NOT NULL, occurrence_id VARCHAR(64) NOT NULL, server_id BIGINT NOT NULL,
 consumer_id VARCHAR(160) NOT NULL, consumer_key VARCHAR(128) NOT NULL, mode VARCHAR(64) NOT NULL,
 start_at BIGINT NOT NULL, end_at BIGINT NOT NULL, claim_end_at BIGINT NOT NULL,
 source_version VARCHAR(64) NOT NULL, form_json LONGBLOB NOT NULL,
 PRIMARY KEY(player_id,occurrence_id), UNIQUE KEY idx_character_activity_consumer(consumer_id),
 UNIQUE KEY idx_character_activity_consumer_key(consumer_key),
 INDEX idx_character_activity_scope(server_id), INDEX idx_character_activity_mode(mode)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
