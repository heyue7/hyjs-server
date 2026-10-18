CREATE TABLE IF NOT EXISTS `p14_final_continuation_offers` (
  `series_id` CHAR(64) NOT NULL,
  `series_manifest_hash` CHAR(64) NOT NULL,
  `policy` LONGBLOB NOT NULL,
  `rules` VARCHAR(1024) NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`series_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
