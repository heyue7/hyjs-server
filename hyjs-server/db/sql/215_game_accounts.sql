-- The runtime initializer qualifies this table with game.accountsDatabase when configured.
CREATE TABLE IF NOT EXISTS game_accounts (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 username VARBINARY(1024) NOT NULL,
 nickname VARCHAR(96) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 password_salt LONGBLOB NOT NULL,
 password_hash LONGBLOB NOT NULL,
 enabled BOOLEAN NOT NULL,
 protected BOOLEAN NOT NULL,
 auth_revision BIGINT NOT NULL DEFAULT 0,
 revision BIGINT NOT NULL,
 created_at BIGINT NOT NULL,
 UNIQUE KEY game_accounts_username (username)
) ENGINE=InnoDB;
