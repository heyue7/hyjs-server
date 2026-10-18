-- P13独立野外队伍实际管理事实。
CREATE TABLE IF NOT EXISTS p13_team_members (player_id BIGINT PRIMARY KEY,team_id VARCHAR(64) NOT NULL,generation BIGINT NOT NULL,joined_at BIGINT NOT NULL,KEY idx_p13_team_members_team(team_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
