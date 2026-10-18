-- 历史记录全部持久化；客户端返回最新100条真实玩家记录。
CREATE TABLE IF NOT EXISTS turntable_records (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  pool_id VARCHAR(64) NOT NULL,
  player_id BIGINT NOT NULL,
  player_name VARCHAR(128) NOT NULL,
  resource_id VARCHAR(64) NOT NULL,
  amount INT NOT NULL,
  jackpot BOOLEAN NOT NULL,
  created_at_millis BIGINT NOT NULL,
  KEY idx_turntable_records_pool(pool_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
