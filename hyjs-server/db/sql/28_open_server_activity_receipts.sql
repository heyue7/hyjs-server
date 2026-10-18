CREATE TABLE IF NOT EXISTS open_server_activity_receipts (
 player_id BIGINT NOT NULL, timestamp BIGINT NOT NULL, request LONGBLOB NOT NULL, response LONGBLOB NOT NULL, PRIMARY KEY(player_id, timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
