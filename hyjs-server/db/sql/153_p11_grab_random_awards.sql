CREATE TABLE IF NOT EXISTS p11_grab_random_awards (
 month VARCHAR(6) NOT NULL,
 player_id BIGINT NOT NULL,
 ordinal BIGINT NOT NULL,
 stamp BIGINT NOT NULL,
 day VARCHAR(8) NOT NULL,
 eligible_millis BIGINT NOT NULL,
 resource_id VARCHAR(64) NOT NULL,
 quantity INT NOT NULL,
 draw BIGINT NOT NULL,
 draw_total BIGINT NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 response LONGBLOB NOT NULL,
 integrity_hash VARCHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(month,player_id,ordinal),
 UNIQUE KEY uq_p11_grab_random_stamp(player_id,stamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
