CREATE TABLE IF NOT EXISTS p13_union_fight_revive_point_sources (
 day CHAR(10) NOT NULL,point_id BIGINT NOT NULL,enabled BOOLEAN NOT NULL DEFAULT FALSE,max_step_millis BIGINT NOT NULL,max_gap_millis BIGINT NOT NULL,published_at BIGINT NOT NULL,source TEXT NOT NULL,PRIMARY KEY(day,point_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
