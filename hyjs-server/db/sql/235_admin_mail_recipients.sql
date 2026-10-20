CREATE TABLE IF NOT EXISTS admin_mail_recipients (
  batch_id VARCHAR(36) NOT NULL,
  player_id BIGINT NOT NULL,
  player_name VARCHAR(191) NOT NULL,
  mail_id BIGINT NOT NULL,
  PRIMARY KEY (batch_id, player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
