CREATE TABLE IF NOT EXISTS account_wallet_balances (
 account_id BIGINT NOT NULL PRIMARY KEY, balance_cents BIGINT NOT NULL, revision BIGINT NOT NULL
) ENGINE=InnoDB;
