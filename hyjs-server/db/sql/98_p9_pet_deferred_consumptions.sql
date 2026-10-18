CREATE TABLE IF NOT EXISTS p9_pet_deferred_consumptions (
 player_id BIGINT NOT NULL, source_key VARCHAR(191) NOT NULL, guid VARCHAR(64) NOT NULL,
 applied BIGINT NOT NULL, capped BIGINT NOT NULL, policy_source LONGTEXT NOT NULL,
 PRIMARY KEY(player_id, source_key, guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
