-- 独立活动配置/状态表。正式环境 InnoDB；由初始化同步补建。
CREATE TABLE IF NOT EXISTS activity_rewards (activity_id VARCHAR(64) NOT NULL, reward_type VARCHAR(32) NOT NULL, threshold INT NOT NULL, required_count INT NOT NULL DEFAULT 0, reward VARCHAR(512) NOT NULL, PRIMARY KEY(activity_id,reward_type,threshold)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
