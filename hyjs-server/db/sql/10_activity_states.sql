-- 独立活动配置/状态表。正式环境 InnoDB；由初始化同步补建。
CREATE TABLE IF NOT EXISTS activity_states (activity_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL, bought BOOLEAN NOT NULL, claims LONGBLOB, PRIMARY KEY(activity_id,player_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
