-- Explicit local first-final draw policy; unpublished/private original randomization is not inferred.
CREATE TABLE IF NOT EXISTS p14_final_draw_policies (
 id BIGINT NOT NULL PRIMARY KEY, enabled TINYINT(1) NOT NULL DEFAULT 0,
 version VARCHAR(128) NOT NULL DEFAULT '', source VARCHAR(1024) NOT NULL,
 published_at BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB;
