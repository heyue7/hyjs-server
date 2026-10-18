CREATE TABLE IF NOT EXISTS p10_social_chat_states (player_id BIGINT NOT NULL, last_send_at BIGINT NOT NULL, PRIMARY KEY (player_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
