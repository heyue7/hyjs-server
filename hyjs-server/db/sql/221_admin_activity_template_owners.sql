CREATE TABLE IF NOT EXISTS admin_activity_template_owners (
 server_id BIGINT NOT NULL, player_id BIGINT NOT NULL, template_id VARCHAR(64) NOT NULL,
 occurrence_id VARCHAR(64) NOT NULL, created_at BIGINT NOT NULL,
 PRIMARY KEY(server_id,player_id,template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
