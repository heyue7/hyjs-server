CREATE TABLE IF NOT EXISTS p9_pet_talent_allocations (
 guid VARCHAR(64) PRIMARY KEY, player_id BIGINT NOT NULL,
 policy_hash VARCHAR(64) NOT NULL, draws LONGBLOB NOT NULL, source LONGTEXT NOT NULL,
 KEY ix_p9_talent_owner(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
