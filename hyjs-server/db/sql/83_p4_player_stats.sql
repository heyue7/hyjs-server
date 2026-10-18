-- P4 独立实例、冻结规则和回执；仅同事务保存成功业务。
CREATE TABLE IF NOT EXISTS p4_player_stats (player_id BIGINT NOT NULL,period VARBINARY(16) NOT NULL,last_power INT NOT NULL,PRIMARY KEY(player_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
