CREATE TABLE IF NOT EXISTS p11_friend_fight_settlements (
 player_id BIGINT NOT NULL,
 stamp BIGINT NOT NULL,
 opponent_id BIGINT NOT NULL,
 fight_id VARCHAR(64) NOT NULL,
 p6_record_id VARCHAR(64) NOT NULL,
 request_hash VARCHAR(64) NOT NULL,
 scope_proof VARCHAR(64) NOT NULL,
 response LONGBLOB NOT NULL,
 response_hash VARCHAR(64) NOT NULL,
 terminal LONGBLOB NOT NULL,
 terminal_hash VARCHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(player_id,stamp),
 UNIQUE KEY p11_friend_fight_id(fight_id),
 UNIQUE KEY p11_friend_p6_id(p6_record_id),
 KEY p11_friend_opponent(opponent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
