CREATE TABLE IF NOT EXISTS opening_calendar_carries (
 player_id BIGINT NOT NULL,
 scope VARCHAR(80) NOT NULL,
 request_id VARCHAR(128) NOT NULL,
 before_start_at BIGINT NOT NULL,
 before_end_at BIGINT NOT NULL,
 before_claim_end_at BIGINT NOT NULL,
 after_start_at BIGINT NOT NULL,
 after_end_at BIGINT NOT NULL,
 after_claim_end_at BIGINT NOT NULL,
 old_now BIGINT NOT NULL,
 real_cutoff BIGINT NOT NULL,
 baseline LONGBLOB NOT NULL,
 login_days LONGBLOB NOT NULL,
 digest VARCHAR(64) NOT NULL,
 PRIMARY KEY (player_id, scope)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
