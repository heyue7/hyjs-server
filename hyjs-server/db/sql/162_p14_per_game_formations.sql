CREATE TABLE IF NOT EXISTS `p14_per_game_formations` (
  `series_id` CHAR(64) NOT NULL,
  `game_index` INT NOT NULL,
  `server_id` BIGINT NOT NULL,
  `player_id` BIGINT NOT NULL,
  `revision` BIGINT NOT NULL,
  `captured_at` BIGINT NOT NULL,
  `fight_power` INT NOT NULL,
  `snapshot` LONGBLOB NOT NULL,
  `battle` LONGBLOB NOT NULL,
  `snapshot_hash` CHAR(64) NOT NULL,
  `configuration_hash` CHAR(64) NOT NULL,
  `engine_version` VARCHAR(128) NOT NULL,
  `client_version` VARCHAR(128) NOT NULL,
  `offer_hash` CHAR(64) NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`series_id`, `game_index`, `server_id`, `player_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
