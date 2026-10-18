-- 原宣战报名独立事实；不生成比赛结果或奖励。
CREATE TABLE IF NOT EXISTS p13_union_fight_settings (guild_id VARCHAR(64) PRIMARY KEY,auto_apply BOOLEAN NOT NULL DEFAULT FALSE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
