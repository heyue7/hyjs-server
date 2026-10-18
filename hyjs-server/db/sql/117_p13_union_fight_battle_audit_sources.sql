CREATE TABLE IF NOT EXISTS p13_union_fight_battle_audit_sources (
 day CHAR(10) NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 seed_salt BIGINT NOT NULL,
 attacker_damage_factor DOUBLE NULL,
 defender_damage_factor DOUBLE NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
