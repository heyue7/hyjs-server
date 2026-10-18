-- 本地服商店：程序首次建立目录；配置热改不被重启重置。
CREATE TABLE IF NOT EXISTS shop_products (
  id VARCHAR(64) PRIMARY KEY,
  source VARCHAR(255) NOT NULL,
  enabled BOOLEAN NOT NULL,
  resource_id VARCHAR(64) NOT NULL,
  amount INT NOT NULL,
  cost VARCHAR(128) NOT NULL,
  daily_limit INT NOT NULL,
  min_level INT NOT NULL,
  min_vip INT NOT NULL,
  priority INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
