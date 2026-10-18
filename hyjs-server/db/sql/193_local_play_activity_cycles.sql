CREATE TABLE IF NOT EXISTS local_play_activity_cycles (
 id VARCHAR(64) NOT NULL PRIMARY KEY, player_id BIGINT NOT NULL, activity_id VARCHAR(64) NOT NULL, created_at BIGINT NOT NULL, real_created_at BIGINT NOT NULL, event_after_id BIGINT NOT NULL, start_at BIGINT NOT NULL, end_at BIGINT NOT NULL, reward_end_at BIGINT NOT NULL, snapshot LONGBLOB NOT NULL, source VARCHAR(64) NOT NULL, KEY idx_local_play_cycle_owner(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
