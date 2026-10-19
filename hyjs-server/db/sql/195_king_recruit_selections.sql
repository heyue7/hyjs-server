-- Explicit role selection; original shared draw/luck/receipt tables remain authoritative.
CREATE TABLE IF NOT EXISTS king_recruit_selections (
 player_id BIGINT NOT NULL,
 pool_id VARCHAR(128) NOT NULL,
 PRIMARY KEY (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
