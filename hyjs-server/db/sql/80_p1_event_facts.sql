-- P1 公共能力独立表；历史来源/领取/截止快照保留，恢复不重复发奖。
CREATE TABLE IF NOT EXISTS p1_event_facts (
 id BIGINT NOT NULL AUTO_INCREMENT, player_id BIGINT NOT NULL, source_key VARBINARY(191) NOT NULL, kind VARBINARY(64) NOT NULL, amount BIGINT NOT NULL, occurred_at BIGINT NOT NULL, day VARCHAR(10) NOT NULL, PRIMARY KEY(id), UNIQUE KEY p1_event_source(player_id,source_key,kind), KEY(player_id), KEY(kind), KEY(occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
