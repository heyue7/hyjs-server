-- 原跨服联合身份、精确回执与持久投递；无胜负和奖励。
CREATE TABLE IF NOT EXISTS p14_import_receipts (id CHAR(64) PRIMARY KEY,payload_hash CHAR(64) NOT NULL,source_server_id BIGINT NOT NULL,imported_at BIGINT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
