CREATE TABLE IF NOT EXISTS p3_payment_luxury_rounds (
  player_id BIGINT NOT NULL,
  offer_id BIGINT NOT NULL,
  start_at BIGINT NOT NULL,
  end_at BIGINT NOT NULL,
  cents BIGINT NOT NULL,
  reward TEXT NOT NULL,
  confirmed_cents BIGINT NOT NULL,
  PRIMARY KEY (player_id, offer_id, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
