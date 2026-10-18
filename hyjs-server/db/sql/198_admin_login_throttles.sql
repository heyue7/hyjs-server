CREATE TABLE IF NOT EXISTS admin_login_throttles (
 username VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin PRIMARY KEY,
 failures BIGINT NOT NULL,
 window_started_at_unix BIGINT NOT NULL,
 locked_until_unix BIGINT NOT NULL
) ENGINE=InnoDB;
