CREATE TABLE IF NOT EXISTS admin_activity_templates (
 id VARCHAR(64) NOT NULL PRIMARY KEY, name VARCHAR(160) NOT NULL, mode VARCHAR(64) NOT NULL,
 form_json LONGBLOB NOT NULL, default_days BIGINT NOT NULL, claim_days BIGINT NOT NULL,
 enabled BOOLEAN NOT NULL, usable BOOLEAN NOT NULL, unavailable_reason TEXT NOT NULL,
 archived BOOLEAN NOT NULL, source_activity_id VARCHAR(160) NOT NULL,
 revision BIGINT NOT NULL, updated_at_millis BIGINT NOT NULL,
 INDEX idx_admin_activity_templates_source_activity_id(source_activity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
