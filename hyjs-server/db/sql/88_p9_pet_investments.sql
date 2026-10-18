CREATE TABLE IF NOT EXISTS p9_pet_investments (
  guid VARCHAR(64) PRIMARY KEY, player_id BIGINT NOT NULL, costs LONGBLOB NOT NULL,
  donors LONGBLOB NOT NULL, source LONGTEXT NOT NULL, KEY ix_p9_investment_owner(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
