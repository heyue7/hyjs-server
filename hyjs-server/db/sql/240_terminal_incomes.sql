CREATE TABLE IF NOT EXISTS terminal_incomes (
 `account_id` BIGINT NOT NULL,
 `id` VARCHAR(128) NOT NULL,
 `label` VARCHAR(255) NOT NULL,
 `amount_cents` BIGINT NOT NULL,
 `created_at` BIGINT NOT NULL,
 `revision` BIGINT NOT NULL,
 `source` VARCHAR(24) NOT NULL,
 PRIMARY KEY (`account_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
