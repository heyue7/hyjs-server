CREATE TABLE IF NOT EXISTS p10_worship_actions (
 player_id BIGINT NOT NULL,
 day VARCHAR(10) NOT NULL,
 target_id BIGINT NOT NULL,
 rank_type INT NOT NULL,
 rank_position BIGINT NOT NULL,
 target_info LONGBLOB NOT NULL,
 daily_hash CHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(player_id,day,target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
