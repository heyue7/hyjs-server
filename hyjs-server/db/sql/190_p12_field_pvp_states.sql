CREATE TABLE IF NOT EXISTS p12_field_pvp_states (
 player_id BIGINT NOT NULL PRIMARY KEY,
 kills INT NOT NULL,
 day VARCHAR(10) NOT NULL,
 daily_kills INT NOT NULL,
 loot_coin BIGINT NOT NULL,
 busy_until BIGINT NOT NULL,
 last_fight_id VARCHAR(64) NOT NULL,
 jail_until BIGINT NOT NULL,
 redeem_count BIGINT NOT NULL,
 source LONGTEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
