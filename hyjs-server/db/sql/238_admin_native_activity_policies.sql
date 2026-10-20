-- 精彩活动仅新增自有策略表；其余配置继续写原有消费者表。
CREATE TABLE IF NOT EXISTS admin_native_activity_policies (
  id VARCHAR(64) PRIMARY KEY,
  form LONGBLOB NOT NULL,
  enabled TINYINT(1) NOT NULL,
  version VARCHAR(64) NOT NULL,
  updated_at BIGINT NOT NULL
) ENGINE=InnoDB;
