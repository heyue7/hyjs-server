CREATE TABLE IF NOT EXISTS p14_audition_rosters (
 season_index INT NOT NULL,
 server_id BIGINT NOT NULL,
 player_id BIGINT NOT NULL,
 revision BIGINT NOT NULL,
 captured_at BIGINT NOT NULL,
 fight_power INT NOT NULL,
 snapshot LONGBLOB NOT NULL,
 battle LONGBLOB NOT NULL,
 snapshot_hash CHAR(64) NOT NULL,
 client_version VARCHAR(128) NOT NULL,
 engine_version VARCHAR(128) NOT NULL,
 configuration_hash CHAR(64) NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,server_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
