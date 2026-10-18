CREATE TABLE IF NOT EXISTS p7_rush_scoped_calculations (
 fight_id VARBINARY(64) NOT NULL PRIMARY KEY,
 player_id BIGINT NOT NULL,
 p6_record_id CHAR(64) NOT NULL,
 series LONGBLOB NOT NULL,
 series_hash CHAR(64) NOT NULL,
 terminal LONGBLOB NOT NULL,
 terminal_hash CHAR(64) NOT NULL,
 start_hash CHAR(64) NOT NULL,
 scope_proof CHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 INDEX idx_p7_scoped_player (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
