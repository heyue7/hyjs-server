-- Named compatibility interpretation; original private probabilities and param semantics are unknown.
CREATE TABLE IF NOT EXISTS p5_gold_egg_rules (
 id VARBINARY(64) PRIMARY KEY NOT NULL,
 enabled BOOLEAN NOT NULL,
 rules LONGBLOB NOT NULL,
 frozen LONGBLOB,
 input_hash CHAR(64) NOT NULL,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
