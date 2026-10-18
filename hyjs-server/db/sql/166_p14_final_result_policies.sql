CREATE TABLE IF NOT EXISTS `p14_final_result_policies` (
  `scope_proof` CHAR(64) NOT NULL,
  `id` BIGINT NOT NULL,
  `enabled` TINYINT(1) NOT NULL,
  `version` VARCHAR(128) NOT NULL,
  `source` VARCHAR(1024) NOT NULL,
  `published_at` BIGINT NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
