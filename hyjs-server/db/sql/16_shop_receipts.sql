-- 本地服商店：程序首次建立目录；配置热改不被重启重置。
CREATE TABLE IF NOT EXISTS shop_receipts (
  player_id BIGINT PRIMARY KEY,
  timestamp BIGINT NOT NULL,
  request LONGBLOB,
  response LONGBLOB
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
