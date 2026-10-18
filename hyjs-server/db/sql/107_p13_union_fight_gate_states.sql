CREATE TABLE IF NOT EXISTS p13_union_fight_gate_states (
 match_id CHAR(64) NOT NULL, guid BIGINT NOT NULL,
 gate_id BIGINT NOT NULL, camp BIGINT NOT NULL,
 frame LONGBLOB NOT NULL, frame_hash CHAR(64) NOT NULL,
 initial_hash CHAR(64) NOT NULL, source TEXT NOT NULL, source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(match_id,guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
