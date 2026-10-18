-- 每名角色每个固定活动独立累计登录，UTC+8日期不重复。
CREATE TABLE IF NOT EXISTS activity_login_states (activity_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL, last_day VARCHAR(10) NOT NULL, days INT NOT NULL, claims LONGBLOB, PRIMARY KEY(activity_id,player_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
