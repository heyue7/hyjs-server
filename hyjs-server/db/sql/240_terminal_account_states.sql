CREATE TABLE IF NOT EXISTS terminal_account_states (
 `account_id` BIGINT NOT NULL,
 `seen_revision` BIGINT NOT NULL,
 `delivered_revision` BIGINT NOT NULL,
 PRIMARY KEY (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
