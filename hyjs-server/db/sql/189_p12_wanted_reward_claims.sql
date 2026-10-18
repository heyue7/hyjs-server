-- P12独立Wanted域；本地规则与原表快照由InitializeP12Wanted冻结，默认关闭待完整资格。
CREATE TABLE IF NOT EXISTS `p12_wanted_reward_claims` (
  `player_id` BIGINT NOT NULL,
  `day` VARCHAR(10) NOT NULL,
  `reward_id` BIGINT NOT NULL,
  `reward` TEXT NOT NULL,
  `response` LONGBLOB NOT NULL,
  `response_hash` VARCHAR(64) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`player_id`,`day`,`reward_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
