-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_luxury_offers` (
  `player_id` BIGINT NOT NULL,
  `offer_id` BIGINT NOT NULL,
  `start_at` BIGINT NOT NULL,
  `end_at` BIGINT NOT NULL,
  `cents` BIGINT NOT NULL,
  `reward` TEXT NOT NULL,
  `claimed` BOOLEAN NOT NULL,
  PRIMARY KEY (`player_id`,`offer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
