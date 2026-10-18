CREATE TABLE IF NOT EXISTS p9_pet_copy_grants (
 player_id BIGINT NOT NULL,
 source_key VARCHAR(191) NOT NULL,
 completed_count BIGINT NOT NULL DEFAULT 1,
 response LONGBLOB NOT NULL,
 facts LONGBLOB NOT NULL,
 created_at BIGINT NOT NULL,
 source LONGTEXT NOT NULL,
 PRIMARY KEY (player_id,source_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
