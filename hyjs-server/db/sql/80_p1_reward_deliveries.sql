-- P1 公共能力独立表；历史来源/领取/截止快照保留，恢复不重复发奖。
CREATE TABLE IF NOT EXISTS p1_reward_deliveries (
 id BIGINT NOT NULL AUTO_INCREMENT, player_id BIGINT NOT NULL, source_key VARBINARY(191) NOT NULL, title VARCHAR(191) NOT NULL, content TEXT NOT NULL, rewards TEXT NOT NULL, created_at BIGINT NOT NULL, expires_at BIGINT NOT NULL, claimed_at BIGINT NOT NULL, result LONGBLOB, PRIMARY KEY(id), UNIQUE KEY p1_reward_source(player_id,source_key), KEY(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
