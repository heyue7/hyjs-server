CREATE TABLE IF NOT EXISTS admin_activity_occurrences (
 id VARCHAR(64) NOT NULL PRIMARY KEY, server_id BIGINT NOT NULL, template_id VARCHAR(64) NOT NULL, template_version VARCHAR(64) NOT NULL,
 name VARCHAR(160) NOT NULL, mode VARCHAR(64) NOT NULL, consumer_key VARCHAR(128) NOT NULL,
 consumer_id VARCHAR(160) NOT NULL, start_at BIGINT NOT NULL, end_at BIGINT NOT NULL,
 claim_end_at BIGINT NOT NULL, lane BIGINT NOT NULL, enabled BOOLEAN NOT NULL,
 archived BOOLEAN NOT NULL, form_json LONGBLOB NOT NULL, revision BIGINT NOT NULL,
 UNIQUE KEY idx_activity_consumer_key(consumer_key), UNIQUE KEY idx_activity_consumer_id(consumer_id),
 INDEX idx_activity_realm_dates(server_id,start_at), INDEX idx_activity_template(template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
