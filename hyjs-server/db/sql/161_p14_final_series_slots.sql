CREATE TABLE IF NOT EXISTS `p14_final_series_slots` (
  `series_id` CHAR(64) NOT NULL,
  `game_index` INT NOT NULL,
  `scheduled_at` BIGINT NOT NULL,
  `lock_at` BIGINT NOT NULL,
  `state` INT NOT NULL,
  `input_id` VARCHAR(64) NOT NULL,
  `input` LONGBLOB NOT NULL,
  `input_hash` VARCHAR(64) NOT NULL,
  `provenance_hash` VARCHAR(64) NOT NULL,
  `certificate` LONGBLOB NOT NULL,
  `certificate_hash` VARCHAR(64) NOT NULL,
  PRIMARY KEY (`series_id`, `game_index`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
