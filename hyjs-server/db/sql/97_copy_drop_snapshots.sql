CREATE TABLE IF NOT EXISTS copy_drop_snapshots (
 player_id BIGINT NOT NULL,
 fight_id VARCHAR(64) NOT NULL,
 task_id VARCHAR(32) NOT NULL,
 input LONGBLOB NOT NULL,
 input_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY (player_id,fight_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
