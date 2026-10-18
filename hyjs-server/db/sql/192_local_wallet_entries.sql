CREATE TABLE IF NOT EXISTS local_wallet_entries (
 player_id BIGINT NOT NULL, `key` VARCHAR(160) NOT NULL,
 kind VARCHAR(16) NOT NULL, delta_cents BIGINT NOT NULL,
 balance_after BIGINT NOT NULL, revision BIGINT NOT NULL,
 order_id VARCHAR(64) NULL, at BIGINT NOT NULL,
 source VARCHAR(64) NOT NULL, reason_id VARCHAR(64) NOT NULL DEFAULT '', label VARCHAR(255) NOT NULL DEFAULT '',
 PRIMARY KEY(player_id, `key`), UNIQUE KEY local_wallet_order(order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
