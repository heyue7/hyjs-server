-- Local first-final formation policy; the original private schedule policy is unknown.
CREATE TABLE IF NOT EXISTS p14_final_formation_policies (
 id BIGINT NOT NULL PRIMARY KEY, enabled TINYINT(1) NOT NULL DEFAULT 0,
 version VARCHAR(128) NOT NULL DEFAULT '', source VARCHAR(1024) NOT NULL,
 published_at BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB;
