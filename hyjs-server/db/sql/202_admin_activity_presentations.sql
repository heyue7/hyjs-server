CREATE TABLE IF NOT EXISTS admin_activity_presentations (
 activity_id VARCHAR(160) PRIMARY KEY,
 mode VARCHAR(64) NOT NULL,
 template_id VARCHAR(96) NOT NULL,
 name VARCHAR(160) NOT NULL,
 archived BOOLEAN NOT NULL,
 updated_at_millis BIGINT NOT NULL
) ENGINE=InnoDB;
