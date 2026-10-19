CREATE TABLE IF NOT EXISTS account_wallet_migrations (
 server_id BIGINT NOT NULL, player_id BIGINT NOT NULL, account_id BIGINT NOT NULL, balance_cents BIGINT NOT NULL, revision BIGINT NOT NULL, entry_count BIGINT NOT NULL, PRIMARY KEY(server_id,player_id)
) ENGINE=InnoDB;
