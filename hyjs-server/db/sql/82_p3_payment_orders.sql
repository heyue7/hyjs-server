-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_orders` (
  `id` VARCHAR(64) NOT NULL,
  `player_id` BIGINT NOT NULL,
  `request_key` BIGINT NOT NULL,
  `request_hash` VARCHAR(64) NOT NULL,
  `product_id` VARCHAR(128) NOT NULL,
  `snapshot` LONGTEXT NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `expires_at` BIGINT NOT NULL,
  `confirmed_at` BIGINT NOT NULL,
  `cents` BIGINT NOT NULL,
  `credited` BIGINT NOT NULL,
  `source` VARCHAR(32) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `p3_order_request` (`player_id`,`request_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
