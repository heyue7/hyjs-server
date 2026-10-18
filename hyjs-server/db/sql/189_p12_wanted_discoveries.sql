-- P12独立Wanted域；本地规则与原表快照由InitializeP12Wanted冻结，默认关闭待完整资格。
CREATE TABLE IF NOT EXISTS `p12_wanted_discoveries` (
  `player_id` BIGINT NOT NULL,
  `source_key` VARCHAR(64) NOT NULL,
  `business_source` VARCHAR(192) NOT NULL,
  `instance_id` VARCHAR(64) NOT NULL,
  `draw` BIGINT NOT NULL,
  `odds` BIGINT NOT NULL,
  `reason` VARCHAR(32) NOT NULL,
  `reward` TEXT NOT NULL,
  `created_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`player_id`,`source_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
