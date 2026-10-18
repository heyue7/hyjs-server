CREATE TABLE IF NOT EXISTS p13_union_fight_car_attack_sources (
 day CHAR(10) NOT NULL,
 factory_id BIGINT NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 hp_percent BIGINT NOT NULL,
 radius DOUBLE NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day,factory_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
