CREATE TABLE IF NOT EXISTS p9_pet_retired (
 guid VARCHAR(64) NOT NULL PRIMARY KEY,
 player_id BIGINT NOT NULL,
 pet_id VARCHAR(64) NOT NULL,
 data LONGBLOB NOT NULL,
 operation VARCHAR(64) NOT NULL,
 timestamp BIGINT NOT NULL,
 coin BIGINT NOT NULL,
 coinpet BIGINT NOT NULL DEFAULT 0,
 source LONGTEXT NOT NULL,
 KEY idx_p9_pet_retired_player_id (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
