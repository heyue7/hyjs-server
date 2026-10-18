-- P13独立持久化事实；原协议字段由service显式映射。
CREATE TABLE IF NOT EXISTS p13_guild_policies (id BIGINT PRIMARY KEY,management_enabled BOOLEAN NOT NULL DEFAULT 0,economy_enabled BOOLEAN NOT NULL DEFAULT 0,chat_enabled BOOLEAN NOT NULL DEFAULT 0,source TEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
