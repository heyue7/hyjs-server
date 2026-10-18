-- P13独立持久化事实；原协议字段由service显式映射。
CREATE TABLE IF NOT EXISTS p13_guild_applies (player_id BIGINT NOT NULL,guild_id VARCHAR(64) NOT NULL,created_at BIGINT NOT NULL,PRIMARY KEY(player_id,guild_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
