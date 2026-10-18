-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_activity_rules` (
  `id` VARCHAR(128) NOT NULL,
  `kind` VARCHAR(32) NOT NULL,
  `start_at` BIGINT NOT NULL,
  `end_at` BIGINT NOT NULL,
  `reward_end_at` BIGINT NOT NULL,
  `threshold` BIGINT NOT NULL,
  `reward` TEXT NOT NULL,
  `name` TEXT NOT NULL,
  `source` TEXT NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
