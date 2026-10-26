CREATE TABLE IF NOT EXISTS p13_union_fight_factory_cycle_sources (
 day char(10) NOT NULL,
 factory_id BIGINT NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 max_generations BIGINT NOT NULL,
 max_alive BIGINT NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 rule VARCHAR(64) NOT NULL DEFAULT '',
 buffer_capacity BIGINT NOT NULL DEFAULT 0,

 PRIMARY KEY(day,factory_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
