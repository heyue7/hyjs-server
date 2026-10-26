CREATE TABLE IF NOT EXISTS ggo_account_weekly (
  account_id BIGINT NOT NULL,
  product_id VARBINARY(64) NOT NULL,
  week_start CHAR(10) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  bought BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, product_id, week_start)
) ENGINE=InnoDB;
