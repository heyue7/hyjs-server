-- P13独立野外队伍实际管理事实。
CREATE TABLE IF NOT EXISTS p13_team_invites (team_id VARCHAR(64) NOT NULL,target_id BIGINT NOT NULL,inviter_id BIGINT NOT NULL,authority_version BIGINT NOT NULL,inviter_generation BIGINT NOT NULL,target_generation BIGINT NOT NULL,created_at BIGINT NOT NULL,PRIMARY KEY(team_id,target_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
