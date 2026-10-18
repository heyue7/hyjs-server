CREATE TABLE IF NOT EXISTS p10_expedition_help_actions (
 owner_id BIGINT NOT NULL,
 mission_serial VARBINARY(128) NOT NULL,
 event_serial VARBINARY(128) NOT NULL,
 helper_id BIGINT NOT NULL,
 day VARCHAR(10) NOT NULL,
 story_id VARCHAR(128) NOT NULL,
 event_hash CHAR(64) NOT NULL,
 friend_since BIGINT NOT NULL,
 reward TEXT NOT NULL,
 daily_hash CHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY (owner_id,mission_serial,event_serial),
 KEY p10_exp_help_day(helper_id,day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
