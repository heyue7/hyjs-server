-- Messages append to history; idempotency keys are scoped to each conversation.
CREATE TABLE IF NOT EXISTS game_feedback_messages (
 id varchar(36) NOT NULL PRIMARY KEY,
 feedback_id varchar(36) NOT NULL,
 request_id varchar(36) NOT NULL,
 sender varchar(16) NOT NULL,
 actor_id bigint NOT NULL,
 content text NOT NULL,
 created_at bigint NOT NULL,
 UNIQUE KEY feedback_message_request(feedback_id,request_id),
 KEY feedback_conversation(feedback_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
