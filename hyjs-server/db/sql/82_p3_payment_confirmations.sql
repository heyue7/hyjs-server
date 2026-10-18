-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_confirmations` (
  `event_id` VARCHAR(128) NOT NULL,
  `order_id` VARCHAR(64) NOT NULL,
  `payload_hash` VARCHAR(64) NOT NULL,
  `outcome` VARCHAR(16) NOT NULL,
  `at` BIGINT NOT NULL,
  `source` VARCHAR(32) NOT NULL,
  PRIMARY KEY (`event_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
