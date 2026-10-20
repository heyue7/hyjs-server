-- Short retry delays for existing game identities; usernames and credentials are not stored.
CREATE TABLE IF NOT EXISTS game_login_throttles (
 account_key VARCHAR(64) PRIMARY KEY,
 failures BIGINT NOT NULL,
 window_started_at BIGINT NOT NULL,
 retry_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 INDEX game_login_updated(updated_at)
) ENGINE=InnoDB;
