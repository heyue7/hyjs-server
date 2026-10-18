-- profiles 行锁保护装备扣除、奖励和同事务回执。
CREATE TABLE IF NOT EXISTS equipment_mutation_receipts (
 player_id BIGINT NOT NULL,
 opcode INT NOT NULL,
 timestamp BIGINT NOT NULL,
 request LONGBLOB,
 response LONGBLOB,
 PRIMARY KEY(player_id, opcode)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
