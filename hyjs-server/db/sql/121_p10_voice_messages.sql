CREATE TABLE IF NOT EXISTS p10_voice_messages (
 id BIGINT NOT NULL AUTO_INCREMENT,asset_id CHAR(32) NOT NULL,owner_id BIGINT NOT NULL,target_id BIGINT NOT NULL DEFAULT 0,chat_type BIGINT NOT NULL,scope VARCHAR(160) NOT NULL,frame LONGBLOB NOT NULL,frame_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,created_at BIGINT NOT NULL,PRIMARY KEY(id),UNIQUE KEY p10_voice_asset(asset_id),KEY p10_voice_sender(owner_id),KEY p10_voice_target(target_id),KEY p10_voice_scope(scope),KEY p10_voice_time(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
