CREATE TABLE IF NOT EXISTS p6_combat_rules (
 id BIGINT NOT NULL PRIMARY KEY, authoritative_enabled BOOLEAN,
 native_capability_hash VARCHAR(64), client_hash VARCHAR(64),
 max_rounds INT, timeout_millis BIGINT, source VARCHAR(255)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
