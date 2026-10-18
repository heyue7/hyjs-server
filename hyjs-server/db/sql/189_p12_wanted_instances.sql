-- P12独立Wanted域；本地规则与原表快照由InitializeP12Wanted冻结，默认关闭待完整资格。
CREATE TABLE IF NOT EXISTS `p12_wanted_instances` (
  `id` VARCHAR(64) NOT NULL,
  `server_id` BIGINT NOT NULL,
  `finder_id` BIGINT NOT NULL,
  `finder_name` VARCHAR(128) NOT NULL,
  `wanted_id` VARCHAR(32) NOT NULL,
  `level` BIGINT NOT NULL,
  `difficulty` BIGINT NOT NULL,
  `state` VARCHAR(16) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `expires_at` BIGINT NOT NULL,
  `shared_at` BIGINT NOT NULL,
  `killed_at` BIGINT NOT NULL,
  `killer_id` BIGINT NOT NULL,
  `current_hp` DOUBLE NOT NULL,
  `max_hp` DOUBLE NOT NULL,
  `monster_hpjson` LONGTEXT NOT NULL,
  `definition_json` LONGTEXT NOT NULL,
  `definition_hash` VARCHAR(64) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_p12_wanted_visible` (`server_id`,`state`,`expires_at`),
  KEY `idx_p12_wanted_owner` (`finder_id`,`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
