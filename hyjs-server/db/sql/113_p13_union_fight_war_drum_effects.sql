CREATE TABLE IF NOT EXISTS p13_union_fight_war_drum_effects (
 match_id VARCHAR(64) NOT NULL,
 guid BIGINT NOT NULL,
 caster_id BIGINT NOT NULL,
 guild_id VARCHAR(64) NOT NULL,
 camp BIGINT NOT NULL,
 x DOUBLE NOT NULL,
 y DOUBLE NOT NULL,
 z DOUBLE NOT NULL,
 started_at BIGINT NOT NULL,
 expires_at BIGINT NOT NULL,
 ended_at BIGINT NOT NULL DEFAULT 0,
 offer_hash VARCHAR(64) NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(match_id,guid),
 INDEX p13_drum_caster(caster_id),
 INDEX p13_drum_pending(ended_at,expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
