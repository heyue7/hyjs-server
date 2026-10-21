CREATE TABLE IF NOT EXISTS terminal_bursts (
 `account_id` BIGINT NOT NULL,
 `request_id` VARCHAR(128) NOT NULL,
 `day` VARCHAR(10) NOT NULL,
 `award_points` BIGINT NOT NULL,
 PRIMARY KEY (`account_id`,`request_id`),
 UNIQUE KEY terminal_burst_day (account_id,day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
