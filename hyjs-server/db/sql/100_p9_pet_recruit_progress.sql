CREATE TABLE IF NOT EXISTS p9_pet_recruit_progress (
 player_id BIGINT PRIMARY KEY, input LONGBLOB NOT NULL, input_hash VARCHAR(64) NOT NULL,
 since_guarantee INT NOT NULL, draws BIGINT NOT NULL, initialized_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
