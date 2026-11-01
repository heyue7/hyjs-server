CREATE TABLE IF NOT EXISTS copy_sweep_witnesses (
  player_id BIGINT NOT NULL,
  max_stamp BIGINT NOT NULL,
  max_digest BINARY(32) NOT NULL,
  last_stamp BIGINT NOT NULL,
  last_digest BINARY(32) NOT NULL,
  last_request LONGBLOB NOT NULL,
  last_response LONGBLOB NOT NULL,
  PRIMARY KEY (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
