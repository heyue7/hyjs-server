CREATE TABLE IF NOT EXISTS gem_mutation_receipts (
 player_id BIGINT NOT NULL,
 opcode INT NOT NULL,
 timestamp BIGINT NOT NULL,
 request LONGBLOB NOT NULL,
 response LONGBLOB NOT NULL,
 PRIMARY KEY (player_id, opcode)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
