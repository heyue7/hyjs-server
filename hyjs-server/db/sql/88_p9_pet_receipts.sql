CREATE TABLE IF NOT EXISTS p9_pet_receipts (
  player_id BIGINT NOT NULL, operation VARCHAR(64) NOT NULL, timestamp BIGINT NOT NULL,
  request LONGBLOB NOT NULL, response LONGBLOB NOT NULL,
  PRIMARY KEY(player_id,operation,timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
