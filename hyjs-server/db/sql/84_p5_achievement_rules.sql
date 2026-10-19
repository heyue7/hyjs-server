-- P5 one-table schema source; no formal account or resource seed.
CREATE TABLE IF NOT EXISTS `p5_achievement_rules` (
  `id` varchar(64) NOT NULL,
  `name` varchar(320) NOT NULL DEFAULT '',
  `type` int NOT NULL DEFAULT 1,
  `disabled` boolean NOT NULL DEFAULT false,
  `kind` varchar(64) NOT NULL,
  `target` int NOT NULL,
  `required_login_days` int NOT NULL DEFAULT 0,
  `required_level` int NOT NULL DEFAULT 0,
  `required_stages` int NOT NULL DEFAULT 0,
  `reward` text NOT NULL,
  `source` text NOT NULL, PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
