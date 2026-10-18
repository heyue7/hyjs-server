-- 由 InitializeTurntable 建表；单行 FOR UPDATE 保护全服共享钻石池和库存。
CREATE TABLE IF NOT EXISTS turntable_globals (
  pool_id VARCHAR(64) NOT NULL PRIMARY KEY,
  diamond_pool BIGINT NOT NULL,
  contribution_remainder BIGINT NOT NULL,
  used_stock LONGBLOB,
  settled BOOLEAN NOT NULL,
  final_ranks LONGBLOB
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
