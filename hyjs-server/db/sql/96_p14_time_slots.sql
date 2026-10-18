CREATE TABLE IF NOT EXISTS p14_time_slots (
  season_index INT NOT NULL,
  start_at BIGINT NOT NULL,
  stage INT NOT NULL,
  end_at BIGINT NOT NULL,
  description TEXT NOT NULL,
  source TEXT NOT NULL,
  PRIMARY KEY(season_index,start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
