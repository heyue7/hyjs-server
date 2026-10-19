CREATE TABLE IF NOT EXISTS account_wallet_entries (
 account_id BIGINT NOT NULL, `key` VARCHAR(240) NOT NULL, server_id BIGINT NOT NULL, player_id BIGINT NOT NULL, legacy_key VARCHAR(160) NOT NULL, kind VARCHAR(16) NOT NULL, delta_cents BIGINT NOT NULL, balance_after BIGINT NOT NULL, revision BIGINT NOT NULL, order_id VARCHAR(64) NULL, at BIGINT NOT NULL, source VARCHAR(64) NOT NULL, reason_id VARCHAR(64) NOT NULL, label VARCHAR(255) CHARACTER SET utf8mb4 NOT NULL, PRIMARY KEY(account_id,`key`), UNIQUE KEY account_wallet_order(order_id)
) ENGINE=InnoDB;
