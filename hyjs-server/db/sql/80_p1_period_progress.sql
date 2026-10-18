-- P1 公共能力独立表；历史来源/领取/截止快照保留，恢复不重复发奖。
CREATE TABLE IF NOT EXISTS p1_period_progress (
 instance_id VARBINARY(64) NOT NULL, player_id BIGINT NOT NULL, snapshot LONGBLOB NOT NULL, value BIGINT NOT NULL, last_day VARCHAR(10) NOT NULL, reward_id BIGINT NOT NULL, PRIMARY KEY(instance_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
