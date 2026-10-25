CREATE TABLE IF NOT EXISTS p7_rush_history_certificates (
 server_id BIGINT NOT NULL,
 player_id BIGINT NOT NULL,
 fight_id VARBINARY(64) NOT NULL,
 format BIGINT NOT NULL,
 validator CHAR(64) NOT NULL,
 fingerprint CHAR(64) NOT NULL,
 outcome_hash CHAR(64) NOT NULL,
 verified_at BIGINT NOT NULL,
 mac CHAR(64) NOT NULL,
 PRIMARY KEY (server_id, player_id, fight_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
