-- P12独立Wanted域；本地规则与原表快照由InitializeP12Wanted冻结，默认关闭待完整资格。
CREATE TABLE IF NOT EXISTS `p12_wanted_policies` (
  `id` BIGINT NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `definition_json` LONGTEXT NOT NULL,
  `definition_hash` VARCHAR(64) NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
