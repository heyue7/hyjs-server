-- Named local score federation; creates no battle, score or qualification.
CREATE TABLE IF NOT EXISTS p14_promotion_rank_offers (
  season_index INT NOT NULL,
  server_id BIGINT NOT NULL,
  version VARCHAR(128) NOT NULL,
  source VARCHAR(1024) NOT NULL,
  rules_json LONGTEXT NOT NULL,
  rules_hash CHAR(64) NOT NULL,
  peer_servers_json TEXT NOT NULL,
  cohort_hash CHAR(64) NOT NULL,
  stage_start BIGINT NOT NULL,
  stage_end BIGINT NOT NULL,
  published_at BIGINT NOT NULL,
  frozen_at BIGINT NOT NULL,
  source_hash CHAR(64) NOT NULL,
  PRIMARY KEY (season_index,server_id)
) ENGINE=InnoDB;
