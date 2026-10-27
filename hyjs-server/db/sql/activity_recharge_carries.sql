CREATE TABLE IF NOT EXISTS activity_recharge_carries (
 instance_id VARCHAR(64) NOT NULL,
 player_id BIGINT NOT NULL,
 request_id VARCHAR(128) NOT NULL,
 real_cutoff BIGINT NOT NULL,
 opening_at BIGINT NOT NULL,
 mode VARCHAR(16) NOT NULL,
 facts LONGBLOB NOT NULL,
 baseline LONGBLOB NOT NULL,
 digest VARCHAR(64) NOT NULL,
 PRIMARY KEY (instance_id, player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
