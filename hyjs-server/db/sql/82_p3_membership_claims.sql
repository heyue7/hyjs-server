-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_membership_claims` (
  `player_id` BIGINT NOT NULL,
  `kind` VARCHAR(128) NOT NULL,
  `period` VARCHAR(32) NOT NULL,
  `reward` TEXT NOT NULL,
  `at` BIGINT NOT NULL,
  PRIMARY KEY (`player_id`,`kind`,`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
