CREATE TABLE IF NOT EXISTS p7_expedition_story_policies (
  id BIGINT NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 0,
  lucky_percent INT NOT NULL,
  source TEXT NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
