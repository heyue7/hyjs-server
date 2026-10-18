CREATE TABLE IF NOT EXISTS p13_union_fight_battle_audit_offers (
 match_id VARCHAR(64) NOT NULL,
 seed_salt BIGINT NOT NULL,
 arrive_range DOUBLE NOT NULL,
 max_rounds INT NOT NULL,
 attacker_damage_factor DOUBLE NULL,
 defender_damage_factor DOUBLE NULL,
 source TEXT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(match_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
