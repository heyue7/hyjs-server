CREATE TABLE IF NOT EXISTS local_play_activity_operations (
 player_id BIGINT NOT NULL, request_id VARCHAR(128) NOT NULL, action VARCHAR(16) NOT NULL, activity_id VARCHAR(64) NOT NULL, response LONGBLOB NOT NULL, created_at BIGINT NOT NULL, PRIMARY KEY(player_id,request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
