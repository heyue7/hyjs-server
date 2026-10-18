CREATE TABLE IF NOT EXISTS p4_popularity_rank_claims (
 player_id BIGINT NOT NULL,
 rule_id VARBINARY(64) NOT NULL,
 product_id VARBINARY(64) NOT NULL,
 `rank` BIGINT NOT NULL,
 rank_proof CHAR(64) NOT NULL,
 offer_hash CHAR(64) NOT NULL,
 source TEXT NOT NULL,
 stamp BIGINT NOT NULL,
 claimed_at BIGINT NOT NULL,
 PRIMARY KEY (player_id,rule_id,product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
