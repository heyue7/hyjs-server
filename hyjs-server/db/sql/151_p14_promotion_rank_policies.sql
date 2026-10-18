-- Named local score federation; creates no battle, score or qualification.
CREATE TABLE IF NOT EXISTS p14_promotion_rank_policies (
  id BIGINT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  version VARCHAR(128) NOT NULL DEFAULT '',
  source VARCHAR(1024) NOT NULL,
  published_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB;
