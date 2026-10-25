CREATE TABLE IF NOT EXISTS field_resource_policies (
  id BIGINT NOT NULL,
  boss_daily_rewards BIGINT NOT NULL DEFAULT -1,
  gather_refresh_hours VARCHAR(128) NOT NULL DEFAULT '[0,12]',
  revision BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
