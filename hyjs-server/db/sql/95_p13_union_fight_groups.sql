-- 真实配对准备事实；日历显式配置，不生成胜负/奖励。
CREATE TABLE IF NOT EXISTS p13_union_fight_groups (week_start VARCHAR(10) NOT NULL,guild_id VARCHAR(64) NOT NULL,`group` CHAR(1) NOT NULL,frozen_power BIGINT NOT NULL,assigned_at BIGINT NOT NULL,source TEXT NOT NULL,PRIMARY KEY(week_start,guild_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
