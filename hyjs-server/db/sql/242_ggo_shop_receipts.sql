CREATE TABLE IF NOT EXISTS ggo_shop_receipts (
 player_id BIGINT NOT NULL,
 request_id VARBINARY(36) NOT NULL,
 product_id VARBINARY(64) NOT NULL,
 version VARBINARY(64) NOT NULL,
 count BIGINT NOT NULL DEFAULT 1,
 response LONGBLOB NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY (player_id, request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
