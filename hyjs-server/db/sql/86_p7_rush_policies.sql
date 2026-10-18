-- P7原Rush协议状态与冻结记录；未通过权威/原内容门禁不消费和发奖。
CREATE TABLE IF NOT EXISTS p7_rush_policies (id BIGINT NOT NULL,enabled BOOLEAN NOT NULL,elite_buy_cost INT NOT NULL,trial_reset_cost INT NOT NULL,source TEXT NOT NULL,PRIMARY KEY(id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
