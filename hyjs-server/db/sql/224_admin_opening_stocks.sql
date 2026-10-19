CREATE TABLE IF NOT EXISTS admin_opening_stocks (
 occurrence_id VARCHAR(64) NOT NULL, task_id INT NOT NULL, used_stock INT NOT NULL,
 PRIMARY KEY(occurrence_id,task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
