CREATE TABLE IF NOT EXISTS p9_pet_instances (
  guid VARCHAR(64) PRIMARY KEY, player_id BIGINT NOT NULL, pet_id VARCHAR(64) NOT NULL,
  data LONGBLOB NOT NULL, created_at BIGINT NOT NULL, source LONGTEXT NOT NULL,
  KEY ix_p9_pet_owner(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
