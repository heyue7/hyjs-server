CREATE TABLE IF NOT EXISTS p7_re_rush_sweeps (
 player_id BIGINT NOT NULL, run_id BIGINT NOT NULL, floor INT NOT NULL,
 star INT NOT NULL, source_fight VARCHAR(64) NOT NULL, source_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(player_id,run_id,floor)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
