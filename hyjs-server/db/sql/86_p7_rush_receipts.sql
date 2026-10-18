-- P7原Rush协议状态与冻结记录；未通过权威/原内容门禁不消费和发奖。
CREATE TABLE IF NOT EXISTS p7_rush_receipts (player_id BIGINT NOT NULL,domain VARBINARY(32) NOT NULL,timestamp BIGINT NOT NULL,request LONGBLOB NOT NULL,response LONGBLOB NOT NULL,PRIMARY KEY(player_id,domain,timestamp)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
