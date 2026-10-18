-- Overrides change only future qualification/grant snapshots of original title identities.
CREATE TABLE IF NOT EXISTS admin_title_overrides (
 id varchar(64) NOT NULL PRIMARY KEY,
 param varchar(64) NOT NULL,
 effects text NOT NULL,
 effect_time bigint NOT NULL,
 `show` boolean NOT NULL,
 enabled boolean NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
