CREATE TABLE IF NOT EXISTS p14_final_continuation_inputs (
 series_id CHAR(64) NOT NULL,
 game_index INT NOT NULL,
 id CHAR(64) NOT NULL,
 offer_hash CHAR(64) NOT NULL,
 previous_result_hash CHAR(64) NOT NULL,
 replay LONGBLOB NOT NULL,
 replay_hash CHAR(64) NOT NULL,
 provenance LONGBLOB NOT NULL,
 provenance_hash CHAR(64) NOT NULL,
 PRIMARY KEY(series_id,game_index),
 UNIQUE KEY idx_p14_continuation_input(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
