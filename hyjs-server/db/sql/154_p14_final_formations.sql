-- Per-round actual profile capture. Original audition/promotion rows remain immutable.
CREATE TABLE IF NOT EXISTS p14_final_formations (
 season_index INT NOT NULL, round_stage INT NOT NULL, server_id BIGINT NOT NULL,
 player_id BIGINT NOT NULL, revision BIGINT NOT NULL, captured_at BIGINT NOT NULL,
 fight_power INT NOT NULL, snapshot LONGBLOB NOT NULL, snapshot_hash CHAR(64) NOT NULL,
 battle LONGBLOB NOT NULL, client_version VARCHAR(128) NOT NULL,
 engine_version VARCHAR(128) NOT NULL, configuration_hash CHAR(64) NOT NULL,
 qualification_hash CHAR(64) NOT NULL, offer_hash CHAR(64) NOT NULL,
 source VARCHAR(1024) NOT NULL, source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,round_stage,server_id,player_id)
) ENGINE=InnoDB;
