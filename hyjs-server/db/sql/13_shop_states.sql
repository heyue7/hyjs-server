-- 本地服商店：程序首次建立目录；配置热改不被重启重置。
CREATE TABLE IF NOT EXISTS shop_states (
  player_id BIGINT NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  day VARCHAR(10) NOT NULL,
  bought INT NOT NULL,
  PRIMARY KEY(player_id,product_id,day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
