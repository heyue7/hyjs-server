CREATE TABLE IF NOT EXISTS terminal_batches (
 `id` VARCHAR(96) NOT NULL,
 `server_id` BIGINT NOT NULL,
 `request_id` VARCHAR(128) NOT NULL,
 `body_hash` VARCHAR(64) NOT NULL,
 `points` BIGINT NOT NULL,
 `reason` VARCHAR(255) NOT NULL,
 `kind` VARCHAR(24) NOT NULL,
 `recipients` BIGINT NOT NULL,
 `credited` BIGINT NOT NULL,
 `created_at` BIGINT NOT NULL,
 `status` VARCHAR(24) NOT NULL,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
