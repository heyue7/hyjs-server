CREATE TABLE IF NOT EXISTS p14_guess_offers (
 season_index INT NOT NULL, guess_type INT NOT NULL, version VARCHAR(128) NOT NULL,
 source VARCHAR(1024) NOT NULL, published_at BIGINT NOT NULL, open_at BIGINT NOT NULL,
 close_at BIGINT NOT NULL, qualification_hash CHAR(64) NOT NULL, draw_hash CHAR(64) NOT NULL,
 rules LONGBLOB NOT NULL, rules_hash CHAR(64) NOT NULL, source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,guess_type)
) ENGINE=InnoDB;
