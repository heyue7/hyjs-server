CREATE TABLE IF NOT EXISTS p12_field_pvp_states (
 player_id BIGINT NOT NULL PRIMARY KEY,
 kills INT NOT NULL,
 day VARCHAR(10) NOT NULL,
 daily_kills INT NOT NULL,
 loot_coin BIGINT NOT NULL,
 busy_until BIGINT NOT NULL,
 clock_version INT NOT NULL DEFAULT 0,
 real_busy_until BIGINT NOT NULL DEFAULT 0,
 last_fight_id VARCHAR(64) NOT NULL,
 jail_until BIGINT NOT NULL,
 real_jail_until BIGINT NOT NULL DEFAULT 0,
 redeem_count BIGINT NOT NULL,
 source LONGTEXT NOT NULL,
 INDEX idx_p12_field_pvp_busy (busy_until),
 INDEX idx_p12_field_pvp_real_busy (real_busy_until)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
