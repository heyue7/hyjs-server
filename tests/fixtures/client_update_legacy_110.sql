-- The schema and active release below model the deployed pre-semantic 1.1.0
-- installation. The real server must upgrade these rows in place.
CREATE TABLE client_update_policy (
  id BIGINT NOT NULL, release_id BIGINT NOT NULL, latest_build BIGINT NOT NULL,
  min_login_build BIGINT NOT NULL, min_run_build BIGINT NOT NULL, revision BIGINT NOT NULL,
  notice_batch BIGINT NOT NULL, notice_enabled BOOLEAN NOT NULL, maintenance BOOLEAN NOT NULL,
  maintenance_start_at BIGINT NOT NULL, maintenance_end_at BIGINT NOT NULL,
  maintenance_epoch BIGINT NOT NULL, login_gate_at BIGINT NOT NULL,
  version_name VARCHAR(80), title VARCHAR(240), content LONGTEXT, mode VARCHAR(32), PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE client_update_releases (
  id BIGINT NOT NULL AUTO_INCREMENT, build_code BIGINT NOT NULL, version_name VARCHAR(80),
  title VARCHAR(240), content LONGTEXT, mode VARCHAR(32), min_login_build BIGINT NOT NULL,
  min_run_build BIGINT NOT NULL, maintenance_start_at BIGINT NOT NULL, maintenance_end_at BIGINT NOT NULL,
  state VARCHAR(32), published_at BIGINT NOT NULL, updated_at BIGINT NOT NULL, published_by VARCHAR(80),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE client_update_sessions (
  player_id BIGINT NOT NULL, session_hash VARCHAR(64), build_code BIGINT NOT NULL, startup_id VARCHAR(64),
  login_at BIGINT NOT NULL, last_seen_at BIGINT NOT NULL, disconnected_at BIGINT NOT NULL,
  maintenance_epoch BIGINT NOT NULL, connected BOOLEAN NOT NULL, PRIMARY KEY (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO client_update_policy VALUES
  (1,1,2026110101,0,0,1,1,FALSE,FALSE,0,0,0,0,'1.1.0','先前版本','旧版记录','notice');
INSERT INTO client_update_releases VALUES
  (1,2026110101,'1.1.0','先前版本','旧版记录','notice',0,0,0,0,'published',1,1,'test');
