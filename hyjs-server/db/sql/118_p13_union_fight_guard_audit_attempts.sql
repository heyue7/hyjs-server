CREATE TABLE IF NOT EXISTS p13_union_fight_guard_audit_attempts (
 match_id VARCHAR(64) NOT NULL,
 guid BIGINT NOT NULL,
 player_id BIGINT NOT NULL,
 guard_guid BIGINT NOT NULL,
 stamp BIGINT NOT NULL,
 request_hash CHAR(64) NOT NULL,
 offer_hash CHAR(64) NOT NULL,
 p6_record_id CHAR(64) NOT NULL,
 input_hash CHAR(64) NOT NULL,
 outcome_hash CHAR(64) NOT NULL,
 replay_hash CHAR(64) NOT NULL,
 refusal VARCHAR(128) NOT NULL,
 source_hash CHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(match_id,guid),
 UNIQUE KEY p13_guard_audit_receipt(match_id,player_id,stamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
