-- MySQL运营配置结构；运行时由同名Gorm模型执行首次建表。
CREATE TABLE IF NOT EXISTS gameplay_pools (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  source VARCHAR(255) NOT NULL,
  enabled BOOLEAN NOT NULL,
  min_level INT NOT NULL,
  single_cost VARCHAR(128) NOT NULL,
  ten_cost VARCHAR(128) NOT NULL,
  single_ticket VARCHAR(128) NOT NULL,
  ten_ticket VARCHAR(128) NOT NULL,
  free_seconds BIGINT NOT NULL,
  free_per_day INT NOT NULL,
  guarantee_every INT NOT NULL,
  coin_per_draw INT NOT NULL,
  start_at BIGINT NOT NULL,
  end_at BIGINT NOT NULL,
  reward_end_at BIGINT NOT NULL,
  initial_diamond_pool BIGINT NOT NULL,
  jackpot_per_million INT NOT NULL,
  pool_contribution_bps INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS gameplay_pool_rewards (
  pool_id VARCHAR(64) NOT NULL,
  id INT NOT NULL,
  resource_id VARCHAR(64) NOT NULL,
  amount INT NOT NULL,
  weight BIGINT NOT NULL,
  guaranteed BOOLEAN NOT NULL,
  stock INT NOT NULL,
  PRIMARY KEY (pool_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
