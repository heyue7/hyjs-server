CREATE TABLE IF NOT EXISTS client_update_releases (
  id BIGINT NOT NULL AUTO_INCREMENT,
  build_code BIGINT NOT NULL,
  version_name VARCHAR(80),
  title VARCHAR(240),
  content LONGTEXT,
  mode VARCHAR(32),
  min_login_build BIGINT NOT NULL,
  min_run_build BIGINT NOT NULL,
  maintenance_start_at BIGINT NOT NULL,
  maintenance_end_at BIGINT NOT NULL,
  state VARCHAR(32),
  published_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  published_by VARCHAR(80),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
