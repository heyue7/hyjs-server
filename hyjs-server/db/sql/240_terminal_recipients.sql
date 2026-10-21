CREATE TABLE IF NOT EXISTS terminal_recipients (
 `batch_id` VARCHAR(96) NOT NULL,
 `account_id` BIGINT NOT NULL,
 `player_id` BIGINT NOT NULL,
 `label` VARCHAR(255) NOT NULL,
 `reason_id` VARCHAR(64) NOT NULL,
 `applied` BOOLEAN NOT NULL,
 PRIMARY KEY (`batch_id`,`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
