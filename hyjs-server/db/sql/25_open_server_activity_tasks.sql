CREATE TABLE IF NOT EXISTS open_server_activity_tasks (
 id INT NOT NULL PRIMARY KEY, title_id INT NOT NULL, task_type INT NOT NULL, tap INT NOT NULL, open_day INT NOT NULL, target INT NOT NULL, visible BOOLEAN NOT NULL, new_price BIGINT NOT NULL, reward TEXT NOT NULL, used_stock INT NOT NULL, source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
