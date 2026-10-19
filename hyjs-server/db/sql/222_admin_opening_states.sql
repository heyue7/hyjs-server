CREATE TABLE IF NOT EXISTS admin_opening_states (
 occurrence_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL, task_id INT NOT NULL,
 achieved BOOLEAN NOT NULL, progress INT NOT NULL, claims INT NOT NULL,
 PRIMARY KEY(occurrence_id,player_id,task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
