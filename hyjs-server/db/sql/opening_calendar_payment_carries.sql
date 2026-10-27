CREATE TABLE IF NOT EXISTS opening_calendar_payment_carries (
 player_id BIGINT NOT NULL,
 scope_id VARCHAR(128) NOT NULL,
 rule_id VARCHAR(64) NOT NULL,
 request_id VARCHAR(128) NOT NULL,
 real_cutoff BIGINT NOT NULL,
 before_start_at BIGINT NOT NULL,
 before_end_at BIGINT NOT NULL,
 before_reward_at BIGINT NOT NULL,
 after_start_at BIGINT NOT NULL,
 after_end_at BIGINT NOT NULL,
 after_reward_at BIGINT NOT NULL,
 facts LONGBLOB NOT NULL,
 baseline LONGBLOB NOT NULL,
 digest VARCHAR(64) NOT NULL,
 PRIMARY KEY (player_id, scope_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
