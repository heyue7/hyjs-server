-- P4 独立实例、冻结规则和回执；仅同事务保存成功业务。
CREATE TABLE IF NOT EXISTS p4_receipts (player_id BIGINT NOT NULL,domain VARBINARY(64) NOT NULL,timestamp BIGINT NOT NULL,request LONGBLOB NOT NULL,response LONGBLOB NOT NULL,PRIMARY KEY(player_id,domain,timestamp)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
