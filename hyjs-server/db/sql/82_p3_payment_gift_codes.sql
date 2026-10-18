-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_gift_codes` (
  `hash` VARCHAR(64) NOT NULL,
  `reward` TEXT NOT NULL,
  `expires_at` BIGINT NOT NULL,
  `limit` BIGINT NOT NULL,
  `used` BIGINT NOT NULL,
  `demo_only` BOOLEAN NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
