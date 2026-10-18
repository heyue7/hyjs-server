-- The first accepted match freezes the explicitly prepublished score policy.
CREATE TABLE IF NOT EXISTS p14_promotion_fight_offers (
  season_index INT NOT NULL,
  server_id BIGINT NOT NULL,
  version VARCHAR(128) NOT NULL,
  source VARCHAR(1024) NOT NULL,
  rules_json LONGTEXT NOT NULL,
  rules_hash CHAR(64) NOT NULL,
  published_at BIGINT NOT NULL,
  captured_at BIGINT NOT NULL,
  source_hash CHAR(64) NOT NULL,
  PRIMARY KEY (season_index, server_id)
) ENGINE=InnoDB;
