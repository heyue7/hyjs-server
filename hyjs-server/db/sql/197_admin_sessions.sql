CREATE TABLE IF NOT EXISTS admin_sessions (
 token_hash CHAR(64) PRIMARY KEY,
 user_id BIGINT NOT NULL,
 csrf_hash CHAR(64) NOT NULL,
 expires_at_unix BIGINT NOT NULL,
 KEY idx_admin_sessions_user_id (user_id),
 KEY idx_admin_sessions_expires_at_unix (expires_at_unix)
) ENGINE=InnoDB;
