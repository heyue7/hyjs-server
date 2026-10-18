CREATE TABLE IF NOT EXISTS p8_gather_receipts (
  player_id BIGINT NOT NULL, operation VARCHAR(32) NOT NULL, timestamp BIGINT NOT NULL,
  request LONGBLOB NOT NULL, response LONGBLOB NOT NULL,
  PRIMARY KEY (player_id, operation, timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
