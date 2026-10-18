-- P1 公共能力独立表；历史来源/领取/截止快照保留，恢复不重复发奖。
CREATE TABLE IF NOT EXISTS p1_mail_messages (
 id BIGINT NOT NULL AUTO_INCREMENT, player_id BIGINT NOT NULL, source_key VARBINARY(191) NOT NULL, mailbox VARCHAR(32) NOT NULL, title VARCHAR(191) NOT NULL, content TEXT NOT NULL, target_id VARCHAR(191) NOT NULL, type INT NOT NULL, created_at BIGINT NOT NULL, expires_at BIGINT NOT NULL, read_at BIGINT NOT NULL, deleted_at BIGINT NOT NULL, reward_id BIGINT NOT NULL, PRIMARY KEY(id), UNIQUE KEY p1_mail_source(player_id,source_key), UNIQUE KEY p1_mail_delete(player_id,type,created_at), KEY(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
