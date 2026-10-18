CREATE TABLE IF NOT EXISTS p14_final_seals (
 season_index INT NOT NULL, round_stage INT NOT NULL, server_id BIGINT NOT NULL,
 content LONGBLOB NOT NULL, content_hash CHAR(64) NOT NULL, received_at BIGINT NOT NULL,
 PRIMARY KEY(season_index,round_stage,server_id)
) ENGINE=InnoDB;
