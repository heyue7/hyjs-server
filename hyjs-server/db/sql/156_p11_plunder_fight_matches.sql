CREATE TABLE IF NOT EXISTS p11_plunder_fight_matches (
 id VARCHAR(64) NOT NULL,
 player_id BIGINT NOT NULL,
 opponent_id BIGINT NOT NULL,
 fragment_id VARCHAR(64) NOT NULL,
 offer_version VARCHAR(64) NOT NULL,
 offer_hash VARCHAR(64) NOT NULL,
 info BLOB NOT NULL,
 attacker LONGBLOB NOT NULL,
 defender LONGBLOB NOT NULL,
 seed BIGINT NOT NULL,
 configuration_hash VARCHAR(64) NOT NULL,
 capability_hash VARCHAR(64) NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 expires_at BIGINT NOT NULL,
 PRIMARY KEY (id),
 KEY idx_p11_plunder_owner (player_id,opponent_id,fragment_id,expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
