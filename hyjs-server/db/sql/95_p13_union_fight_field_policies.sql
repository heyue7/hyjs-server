CREATE TABLE IF NOT EXISTS p13_union_fight_field_policies (
 guard_rule VARCHAR(64) NOT NULL DEFAULT '',
 guard_hp_source VARCHAR(32) NOT NULL DEFAULT '',
 map_projection VARCHAR(32) NOT NULL DEFAULT '',
 id BIGINT NOT NULL,enabled BOOLEAN NOT NULL DEFAULT FALSE,preparation_millis BIGINT NOT NULL DEFAULT 0,
 left_revive_point_id BIGINT NOT NULL DEFAULT 0,right_revive_point_id BIGINT NOT NULL DEFAULT 0,
 initial_guard_hp DOUBLE NOT NULL DEFAULT 0,initial_gate_hp DOUBLE NOT NULL DEFAULT 0,move_tolerance DOUBLE NOT NULL DEFAULT 0,
 source TEXT NOT NULL,PRIMARY KEY(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
