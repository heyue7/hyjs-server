CREATE TABLE IF NOT EXISTS `p14_per_game_offers` (
  `series_id` CHAR(64) NOT NULL,
  `game_index` INT NOT NULL,
  `series_manifest_hash` CHAR(64) NOT NULL,
  `policy` LONGBLOB NOT NULL,
  `open_at` BIGINT NOT NULL,
  `lock_at` BIGINT NOT NULL,
  `scheduled_at` BIGINT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`series_id`, `game_index`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
