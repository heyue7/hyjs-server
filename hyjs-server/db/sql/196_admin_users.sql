CREATE TABLE IF NOT EXISTS admin_users (
 id BIGINT PRIMARY KEY AUTO_INCREMENT,
 username VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 password_hash VARCHAR(128) NOT NULL,
 role VARCHAR(16) NOT NULL,
 enabled BOOLEAN NOT NULL,
 created_at_unix BIGINT NOT NULL,
 UNIQUE KEY idx_admin_users_username (username)
) ENGINE=InnoDB;
