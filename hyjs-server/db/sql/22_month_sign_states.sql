CREATE TABLE IF NOT EXISTS month_sign_states (
 player_id BIGINT NOT NULL, month VARCHAR(7) NOT NULL, days INT NOT NULL,
 last_day VARCHAR(10) NOT NULL, doubled BOOLEAN NOT NULL,
 last_reward TEXT NOT NULL, last_double_vip INT NOT NULL,
 PRIMARY KEY(player_id, month)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
