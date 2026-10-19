CREATE TABLE IF NOT EXISTS game_account_roles (
 account_id BIGINT NOT NULL,
 server_id BIGINT NOT NULL,
 player_id BIGINT NOT NULL,
 name VARCHAR(96) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 level BIGINT NOT NULL,
 PRIMARY KEY (account_id, server_id)
) ENGINE=InnoDB;
