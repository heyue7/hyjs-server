CREATE TABLE IF NOT EXISTS p7_maze_trader_offers (
 player_id BIGINT NOT NULL,
 run_id BIGINT NOT NULL,
 fight_id VARCHAR(64) NOT NULL,
 start_hash VARCHAR(64) NOT NULL,
 result_hash VARCHAR(64) NOT NULL,
 policy LONGBLOB NOT NULL,
 policy_hash VARCHAR(64) NOT NULL,
 draw INT NOT NULL,
 appeared BOOLEAN NOT NULL,
 frozen_at BIGINT NOT NULL,
 purchased_at BIGINT NOT NULL DEFAULT 0,
 buy_stamp BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(player_id,run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
