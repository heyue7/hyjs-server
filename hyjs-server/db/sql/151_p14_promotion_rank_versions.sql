-- Named local score federation; creates no battle, score or qualification.
CREATE TABLE IF NOT EXISTS p14_promotion_rank_versions (
  season_index INT NOT NULL,
  server_id BIGINT NOT NULL,
  revision BIGINT NOT NULL,
  content LONGBLOB NOT NULL,
  content_hash CHAR(64) NOT NULL,
  previous_hash CHAR(64) NOT NULL,
  captured_at BIGINT NOT NULL,
  imported_at BIGINT NOT NULL,
  PRIMARY KEY (season_index,server_id,revision)
) ENGINE=InnoDB;
