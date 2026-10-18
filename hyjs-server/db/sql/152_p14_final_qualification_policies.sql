-- Explicit local sparse-final rule. Publish before original promotion stage6; default closed.
CREATE TABLE IF NOT EXISTS p14_final_qualification_policies (
 id BIGINT NOT NULL, enabled TINYINT(1) NOT NULL DEFAULT 0, version VARCHAR(128) NOT NULL DEFAULT '', source VARCHAR(1024) NOT NULL, published_at BIGINT NOT NULL DEFAULT 0, PRIMARY KEY(id)
) ENGINE=InnoDB;
