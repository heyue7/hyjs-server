-- Stable catalogue gates; P1 owns these rows for the life of the database.
CREATE TABLE IF NOT EXISTS p1_gather_catalog_guards (
 kind VARBINARY(64) NOT NULL,
 PRIMARY KEY(kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
