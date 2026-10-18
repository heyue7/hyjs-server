-- 独立活动配置/状态表。正式环境 InnoDB；由初始化同步补建。
CREATE TABLE IF NOT EXISTS activity_rules (id VARCHAR(64) PRIMARY KEY, enabled BOOLEAN NOT NULL, min_vip INT NOT NULL, diamond_cost INT NOT NULL, start_at BIGINT NOT NULL, end_at BIGINT NOT NULL, reward_end_at BIGINT NOT NULL, source TEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
