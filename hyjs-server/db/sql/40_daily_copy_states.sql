CREATE TABLE IF NOT EXISTS daily_copy_states (
 player_id BIGINT NOT NULL, day VARCHAR(10) NOT NULL, type VARCHAR(64) NOT NULL,
 used INT NOT NULL, `limit` INT NOT NULL,
 PRIMARY KEY(player_id,day,type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
