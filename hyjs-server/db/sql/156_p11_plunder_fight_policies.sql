CREATE TABLE IF NOT EXISTS p11_plunder_fight_policies (
 id BIGINT NOT NULL,
 enabled BOOLEAN NOT NULL,
 permanent BOOLEAN NOT NULL DEFAULT FALSE,
  sweep_enabled TINYINT(1) NOT NULL DEFAULT 0,
 version VARCHAR(64) NOT NULL,
 published_at BIGINT NOT NULL,
 start_at BIGINT NOT NULL,
 end_at BIGINT NOT NULL,
 match_count INT NOT NULL,
 scan_limit INT NOT NULL,
 level_difference INT NOT NULL,
 match_millis BIGINT NOT NULL,
 chance_tier INT NOT NULL,
 drop_ppm BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
