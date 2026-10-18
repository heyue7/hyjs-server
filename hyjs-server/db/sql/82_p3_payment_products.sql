-- P3独立支付域；原规则来源及快照由InitializePayments插入。
CREATE TABLE IF NOT EXISTS `p3_payment_products` (
  `id` VARCHAR(128) NOT NULL,
  `channels` TEXT NOT NULL,
  `shop` VARCHAR(32) NOT NULL,
  `kind` VARCHAR(32) NOT NULL,
  `cents` BIGINT NOT NULL,
  `currency` VARCHAR(8) NOT NULL,
  `charge` BIGINT NOT NULL,
  `free` BIGINT NOT NULL,
  `reward` TEXT NOT NULL,
  `name` TEXT NOT NULL,
  `description` TEXT NOT NULL,
  `source` TEXT NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `first_double` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
