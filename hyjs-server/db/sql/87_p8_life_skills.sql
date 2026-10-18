CREATE TABLE IF NOT EXISTS p8_life_skills (
  player_id BIGINT NOT NULL, skill_id VARCHAR(64) NOT NULL, book_id VARCHAR(64) NOT NULL,
  learned_at BIGINT NOT NULL, source LONGTEXT NOT NULL,
  PRIMARY KEY (player_id, skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
