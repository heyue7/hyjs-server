CREATE TABLE IF NOT EXISTS p13_union_fight_speed_item_effects (
 match_id VARCHAR(64) NOT NULL,
 guid BIGINT NOT NULL,
 target_id BIGINT NOT NULL,
 caster_id BIGINT NOT NULL,
 started_at BIGINT NOT NULL,
 expires_at BIGINT NOT NULL,
 ended_at BIGINT NOT NULL DEFAULT 0,
 multiplier DOUBLE NOT NULL,
 offer_hash VARCHAR(64) NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(match_id,guid),
 INDEX p13_speed_target(target_id),
 INDEX p13_speed_pending(ended_at,expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
