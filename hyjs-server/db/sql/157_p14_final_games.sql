CREATE TABLE IF NOT EXISTS p14_final_games (
 id CHAR(64) NOT NULL PRIMARY KEY, season_index INT NOT NULL, round_stage INT NOT NULL,
 group_id CHAR(1) NOT NULL, pair_index INT NOT NULL, game_index INT NOT NULL,
 replay LONGBLOB NOT NULL, replay_hash CHAR(64) NOT NULL,
 provenance LONGBLOB NOT NULL, provenance_hash CHAR(64) NOT NULL, created_at BIGINT NOT NULL,
 UNIQUE KEY idx_p14_final_game_pair(season_index,round_stage,group_id,pair_index,game_index)
) ENGINE=InnoDB;
