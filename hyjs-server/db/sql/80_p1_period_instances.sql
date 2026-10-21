-- P1 公共能力独立表；历史来源/领取/截止快照保留，恢复不重复发奖。
CREATE TABLE IF NOT EXISTS p1_period_instances (
 id VARBINARY(64) NOT NULL, reward_source_id VARCHAR(64) NOT NULL DEFAULT '', enabled BOOLEAN NOT NULL, title VARCHAR(191) NOT NULL, kind VARBINARY(64) NOT NULL, mode VARCHAR(16) NOT NULL, goal BIGINT NOT NULL, rewards TEXT NOT NULL, milestones LONGBLOB, start_at BIGINT NOT NULL, end_at BIGINT NOT NULL, reward_end_at BIGINT NOT NULL, rank_places INT NOT NULL, rank_rewards TEXT NOT NULL, frozen LONGBLOB, source TEXT NOT NULL, PRIMARY KEY(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
