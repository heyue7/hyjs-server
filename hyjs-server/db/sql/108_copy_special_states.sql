CREATE TABLE IF NOT EXISTS copy_special_states (
 player_id BIGINT NOT NULL,
 task_id VARBINARY(32) NOT NULL,
 reward_id VARBINARY(64) NOT NULL,
 credit BIGINT NOT NULL,
 successes BIGINT NOT NULL,
 spent BIGINT NOT NULL,
 granted BIGINT NOT NULL,
 last_input_hash CHAR(64) NOT NULL,
 PRIMARY KEY(player_id,task_id,reward_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
