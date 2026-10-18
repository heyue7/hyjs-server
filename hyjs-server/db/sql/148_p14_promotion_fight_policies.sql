-- The compatibility policy stays closed until its versioned local score rules are published.
CREATE TABLE IF NOT EXISTS p14_promotion_fight_policies (
  id BIGINT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  version VARCHAR(128) NOT NULL DEFAULT '',
  source VARCHAR(1024) NOT NULL,
  rules_json LONGTEXT NOT NULL,
  rules_hash CHAR(64) NOT NULL DEFAULT '',
  published_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB;
