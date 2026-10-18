-- APK 未提供积分和排名奖励；下列规则是本地服运营配置。
CREATE TABLE IF NOT EXISTS turntable_rules (
  pool_id VARCHAR(64) NOT NULL PRIMARY KEY,
  score_per_draw INT NOT NULL,
  top_prize VARCHAR(128) NOT NULL,
  advance_prize VARCHAR(128) NOT NULL,
  advance_places INT NOT NULL,
  description TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
