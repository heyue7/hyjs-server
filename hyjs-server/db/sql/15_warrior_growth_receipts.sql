CREATE TABLE IF NOT EXISTS warrior_growth_receipts (
 player_id BIGINT NOT NULL,
 guid VARCHAR(128) NOT NULL,
 kind VARCHAR(32) NOT NULL,
 timestamp BIGINT NOT NULL,
 request BLOB NOT NULL,
 response BLOB NOT NULL,
 PRIMARY KEY(player_id,guid,kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
