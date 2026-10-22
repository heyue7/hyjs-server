CREATE TABLE IF NOT EXISTS turntable_states (
  prize_pity_state TEXT,
  pool_id VARCHAR(64) NOT NULL,
  player_id BIGINT NOT NULL,
  player_name VARCHAR(128) NOT NULL,
  score INT NOT NULL,
  bought_coins BIGINT NOT NULL,
  next_free BIGINT NOT NULL,
  free_day VARCHAR(10) NOT NULL,
  free_used INT NOT NULL,
  timestamp BIGINT NOT NULL,
  request_count INT NOT NULL,
  response LONGBLOB,
  PRIMARY KEY (pool_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
