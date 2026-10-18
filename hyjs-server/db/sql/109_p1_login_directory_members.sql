-- Stable original Type4 indices reuse existing period progress and reward deliveries.
CREATE TABLE IF NOT EXISTS p1_login_directory_members (
 index_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 instance_id VARBINARY(64) NOT NULL,
 child_index INT NOT NULL,
 UNIQUE KEY idx_p1_login_directory_binding (instance_id,child_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
