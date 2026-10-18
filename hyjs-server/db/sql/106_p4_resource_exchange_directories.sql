-- A visible original Type23 entry maps to frozen child indices; quotas stay in p4_commerce_states.
CREATE TABLE IF NOT EXISTS p4_resource_exchange_directories (
 id VARBINARY(64) NOT NULL PRIMARY KEY,
 enabled BOOLEAN NOT NULL,
 start_at BIGINT NOT NULL,
 end_at BIGINT NOT NULL,
 bindings LONGBLOB NOT NULL,
 input_hash CHAR(64) NOT NULL,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
