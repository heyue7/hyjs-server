-- Only the permanent gain magnitude below is a local policy; no recovered APK value specifies it.
CREATE TABLE IF NOT EXISTS cooking_rules (
 id BIGINT NOT NULL PRIMARY KEY,
 enabled BOOLEAN NOT NULL,
 gains TEXT NOT NULL,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
