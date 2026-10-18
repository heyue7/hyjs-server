CREATE TABLE IF NOT EXISTS p9_pet_states (
  player_id BIGINT PRIMARY KEY, first_granted BOOLEAN NOT NULL, next_free BIGINT NOT NULL,
  free_remaining INT NOT NULL, must_times INT NOT NULL, guard_guid VARCHAR(64) NOT NULL,
  source LONGTEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
