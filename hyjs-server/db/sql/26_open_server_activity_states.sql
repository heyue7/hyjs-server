CREATE TABLE IF NOT EXISTS open_server_activity_states (
 player_id BIGINT NOT NULL, task_id INT NOT NULL, achieved BOOLEAN NOT NULL, progress INT NOT NULL, claims INT NOT NULL, PRIMARY KEY(player_id, task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
