-- Permanent combined-page indices are never reassigned or removed.
CREATE TABLE IF NOT EXISTS recharge_directory_members (
 index_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 domain VARBINARY(8) NOT NULL,
 instance_id VARBINARY(512) NOT NULL,
 child_index INT NOT NULL,
 UNIQUE KEY idx_recharge_directory_binding (domain,instance_id,child_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
