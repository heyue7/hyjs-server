CREATE TABLE IF NOT EXISTS activity_calendar_period_archives (
 instance_id VARBINARY(64) NOT NULL,
 request_id VARCHAR(128) NOT NULL,
 settled_at BIGINT NOT NULL,
 snapshot LONGBLOB NOT NULL,
 before_start_at BIGINT NOT NULL,
 before_end_at BIGINT NOT NULL,
 before_claim_end_at BIGINT NOT NULL,
 after_start_at BIGINT NOT NULL,
 after_end_at BIGINT NOT NULL,
 after_claim_end_at BIGINT NOT NULL,
 digest VARCHAR(64) NOT NULL,
 PRIMARY KEY (instance_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
