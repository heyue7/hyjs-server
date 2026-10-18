CREATE TABLE IF NOT EXISTS p12_world_boss_battle_participations (
  player_id BIGINT NOT NULL PRIMARY KEY,
  remaining BIGINT NOT NULL,
  recovered_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
