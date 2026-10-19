CREATE TABLE IF NOT EXISTS p4_popularity_rank_offer_history (
 offer_hash CHAR(64) NOT NULL,
 rule_id VARBINARY(64) NOT NULL,
 payload LONGBLOB NOT NULL,
 PRIMARY KEY (offer_hash),
 KEY rank_offer_rule (rule_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
