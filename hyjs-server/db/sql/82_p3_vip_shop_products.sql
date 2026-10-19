-- P3本地会员商店档；原shopType2，未知运营目录明确记录为本地规则。
CREATE TABLE IF NOT EXISTS `p3_vip_shop_products` (
 `id` VARCHAR(64) NOT NULL,
 `min_vip` BIGINT NOT NULL,
 `cost` BIGINT NOT NULL,
 `cost_resource` VARCHAR(64) NOT NULL DEFAULT '',
 `reward` TEXT NOT NULL,
 `source` TEXT NOT NULL,
 `enabled` BOOLEAN NOT NULL,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
