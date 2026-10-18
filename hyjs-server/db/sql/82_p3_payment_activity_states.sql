-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_activity_states` (
  `player_id` BIGINT NOT NULL,
  `activity_id` VARCHAR(128) NOT NULL,
  `snapshot` LONGTEXT NOT NULL,
  `claimed` BOOLEAN NOT NULL,
  PRIMARY KEY (`player_id`,`activity_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
