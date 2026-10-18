-- 真实配对准备事实；日历显式配置，不生成胜负/奖励。
CREATE TABLE IF NOT EXISTS p13_union_fight_schedules (day VARCHAR(10) PRIMARY KEY,first_start_at BIGINT NOT NULL,second_start_at BIGINT NOT NULL,duration_millis BIGINT NOT NULL,source TEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
