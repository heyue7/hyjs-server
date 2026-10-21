CREATE TABLE IF NOT EXISTS client_update_actions (
  request_id VARCHAR(80) NOT NULL,
  payload_hash VARCHAR(64),
  actor_id BIGINT NOT NULL,
  response_json LONGTEXT,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
