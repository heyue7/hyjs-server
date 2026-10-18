CREATE TABLE IF NOT EXISTS p13_union_fight_chats (
 id BIGINT AUTO_INCREMENT PRIMARY KEY,
 match_id CHAR(64) NOT NULL,
 guild_id VARCHAR(64) NOT NULL,
 kind BIGINT NOT NULL,
 sender_id BIGINT NOT NULL,
 created_at BIGINT NOT NULL,
 request BLOB NOT NULL,
 frame BLOB NOT NULL,
 frame_hash CHAR(64) NOT NULL,
 source TEXT NOT NULL,
 KEY p13_uf_chat_scope (match_id,guild_id,kind),
 KEY p13_uf_chat_sender (sender_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
