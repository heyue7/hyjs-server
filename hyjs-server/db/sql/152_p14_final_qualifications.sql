-- Immutable complete source set. No score/battle/bye/winner is produced by this table.
CREATE TABLE IF NOT EXISTS p14_final_qualifications (season_index INT NOT NULL, content LONGBLOB NOT NULL, content_hash CHAR(64) NOT NULL, observed_at BIGINT NOT NULL, PRIMARY KEY(season_index)) ENGINE=InnoDB;
