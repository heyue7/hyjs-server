-- Named local cadence; source triples are audit metadata, original probabilities unknown.
CREATE TABLE IF NOT EXISTS copy_special_rules (
 id VARBINARY(64) PRIMARY KEY NOT NULL,
 enabled BOOLEAN NOT NULL,
 cycle BIGINT NOT NULL,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
