CREATE TABLE IF NOT EXISTS p7_expedition_events (
 serial VARBINARY(64) NOT NULL PRIMARY KEY,
 mission_serial VARBINARY(64) NOT NULL,
 player_id BIGINT NOT NULL,
 story_id VARCHAR(64) NOT NULL,
 kind INT NOT NULL,
 event_at BIGINT NOT NULL,
 reward TEXT NOT NULL,
 resolved_at BIGINT NOT NULL,
 helper_id BIGINT NOT NULL,
 parent_serial VARBINARY(64) NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 KEY idx_p7_exp_event_owner(player_id,mission_serial)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
