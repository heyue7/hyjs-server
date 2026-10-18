-- Exact common qualification and original first-final calendar; no result fields.
CREATE TABLE IF NOT EXISTS p14_final_formation_offers (
 season_index INT NOT NULL, round_stage INT NOT NULL, version VARCHAR(128) NOT NULL,
 source VARCHAR(1024) NOT NULL, published_at BIGINT NOT NULL,
 qualification_hash CHAR(64) NOT NULL, stage_start BIGINT NOT NULL,
 fight_start BIGINT NOT NULL, fight_end BIGINT NOT NULL, lock_at BIGINT NOT NULL,
 source_hash CHAR(64) NOT NULL, PRIMARY KEY(season_index,round_stage)
) ENGINE=InnoDB;
