-- Real finalized entrant identities and real ordered pairs only; no winner, bye or game result.
CREATE TABLE IF NOT EXISTS p14_final_draws (
 season_index INT NOT NULL PRIMARY KEY, content LONGBLOB NOT NULL,
 content_hash CHAR(64) NOT NULL, observed_at BIGINT NOT NULL
) ENGINE=InnoDB;
