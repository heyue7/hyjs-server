CREATE TABLE IF NOT EXISTS terminal_settings (
 `server_id` BIGINT NOT NULL,
 `payload` LONGTEXT NOT NULL,
 PRIMARY KEY (`server_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
