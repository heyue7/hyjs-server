CREATE TABLE IF NOT EXISTS p7_re_rush_sections (
 player_id BIGINT NOT NULL, run_id BIGINT NOT NULL, section INT NOT NULL,
 claimed BOOLEAN NOT NULL DEFAULT FALSE, selected VARCHAR(64) NOT NULL DEFAULT '',
 spent_stars INT NOT NULL DEFAULT 0, acceptance_hash VARCHAR(64) NOT NULL DEFAULT '',
 PRIMARY KEY(player_id,run_id,section)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
