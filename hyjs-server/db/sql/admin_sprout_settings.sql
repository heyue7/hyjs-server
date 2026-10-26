CREATE TABLE IF NOT EXISTS admin_sprout_settings (
 id BIGINT NOT NULL PRIMARY KEY,
 enabled BOOLEAN NOT NULL DEFAULT FALSE,
 days BIGINT NULL,
 new_character_enabled BOOLEAN NULL,
 return_enabled BOOLEAN NULL,
 return_offline_days BIGINT NULL,
 return_days BIGINT NULL,
 revision BIGINT NOT NULL DEFAULT 1,
 updated_at BIGINT NOT NULL DEFAULT 0,
 updated_by VARCHAR(64) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
