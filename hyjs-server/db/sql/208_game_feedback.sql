-- Conversations retain the original player owner and immutable create request identity.
CREATE TABLE IF NOT EXISTS game_feedback (
 id varchar(36) NOT NULL PRIMARY KEY,
 player_id bigint NOT NULL,
 request_id varchar(36) NOT NULL,
 owner_id bigint NOT NULL,
 category varchar(16) NOT NULL,
 subject varchar(320) NOT NULL,
 state varchar(16) NOT NULL,
 created_at bigint NOT NULL,
 updated_at bigint NOT NULL,
 UNIQUE KEY feedback_request(owner_id,request_id),
 KEY feedback_player(player_id),
 KEY feedback_state(state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
