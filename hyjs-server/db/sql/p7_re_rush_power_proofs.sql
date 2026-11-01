CREATE TABLE IF NOT EXISTS p7_re_rush_power_proofs (
 player_id BIGINT NOT NULL, run_id BIGINT NOT NULL, floor INT NOT NULL,
 snapshot LONGBLOB NOT NULL, snapshot_hash CHAR(64) NOT NULL,
 PRIMARY KEY(player_id,run_id,floor),
 KEY idx_p7_power_floor(player_id,floor,run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
