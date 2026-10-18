CREATE TABLE IF NOT EXISTS `p14_final_settlement_policies` (
  `id` BIGINT NOT NULL,
  `scope_proof` CHAR(64) NOT NULL,
  `enabled` TINYINT(1) NOT NULL,
  `version` VARCHAR(128) NOT NULL,
  `source` VARCHAR(1024) NOT NULL,
  `published_at` BIGINT NOT NULL,
  `wrapper_rule` VARCHAR(128) NOT NULL,
  `wrapper_manifest_hash` CHAR(64) NOT NULL,
  `wrapper_dll_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
