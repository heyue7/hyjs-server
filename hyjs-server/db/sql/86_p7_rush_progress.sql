-- P7原Rush协议状态与冻结记录；未通过权威/原内容门禁不消费和发奖。
CREATE TABLE IF NOT EXISTS p7_rush_progress (player_id BIGINT NOT NULL,kind INT NOT NULL,task_id VARCHAR(64) NOT NULL,star INT NOT NULL,first_fight VARBINARY(64) NOT NULL,PRIMARY KEY(player_id,kind,task_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
