-- Named local score federation; creates no battle, score or qualification.
CREATE TABLE IF NOT EXISTS p14_promotion_rank_deliveries (
  id CHAR(64) NOT NULL,
  season_index INT NOT NULL,
  source_server_id BIGINT NOT NULL,
  destination_server_id BIGINT NOT NULL,
  revision BIGINT NOT NULL,
  payload LONGBLOB NOT NULL,
  payload_hash CHAR(64) NOT NULL,
  confirmed BOOLEAN NOT NULL DEFAULT FALSE,
  attempt_count BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  confirmed_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY p14_rank_pending(season_index,destination_server_id,revision,confirmed)
) ENGINE=InnoDB;
