-- 原始首次掉落与额外投递沿用调用方角色行锁事务，同来源仅投递一次。
CREATE TABLE IF NOT EXISTS p4_copy_benefit_grants (player_id BIGINT NOT NULL,source_key VARBINARY(128) NOT NULL,rule_id VARBINARY(64) NOT NULL,base_hash CHAR(64) NOT NULL,bonus LONGBLOB NOT NULL,PRIMARY KEY(player_id,source_key)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
