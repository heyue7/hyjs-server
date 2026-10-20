CREATE TABLE IF NOT EXISTS admin_security_events (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 event_key CHAR(64) NOT NULL,
 first_at BIGINT NOT NULL,
 last_at BIGINT NOT NULL,
 actor VARCHAR(64) NOT NULL,
 source_ip VARCHAR(64) NOT NULL,
 action VARCHAR(64) NOT NULL,
 target VARCHAR(191) NOT NULL,
 outcome VARCHAR(16) NOT NULL,
 reason_code VARCHAR(64) NOT NULL,
 request_id VARCHAR(64) NOT NULL,
 occurrences BIGINT NOT NULL,
 UNIQUE KEY idx_admin_security_events_event_key (event_key),
 KEY idx_admin_security_events_last_at (last_at),
 KEY idx_admin_security_events_action (action),
 KEY idx_admin_security_events_outcome (outcome)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
