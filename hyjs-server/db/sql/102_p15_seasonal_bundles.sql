-- 原节日/新版页按父activityId查询嵌套列表，历史目录独立冻结。
CREATE TABLE IF NOT EXISTS p15_seasonal_bundles (
 id VARBINARY(64) PRIMARY KEY, kind VARCHAR(16) NOT NULL, title VARCHAR(191) NOT NULL,
 enabled BOOLEAN NOT NULL, start_at BIGINT NOT NULL, end_at BIGINT NOT NULL,
 reward_end_at BIGINT NOT NULL, children LONGBLOB NOT NULL, frozen LONGBLOB, source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
