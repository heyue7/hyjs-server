CREATE TABLE IF NOT EXISTS `p14_per_game_seals` (
  `series_id` CHAR(64) NOT NULL,
  `game_index` INT NOT NULL,
  `server_id` BIGINT NOT NULL,
  `content` LONGBLOB NOT NULL,
  `content_hash` CHAR(64) NOT NULL,
  `sealed_at` BIGINT NOT NULL,
  PRIMARY KEY (`series_id`, `game_index`, `server_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
