CREATE TABLE IF NOT EXISTS `p13_union_fight_settlement_policies` (
 abort_rule VARCHAR(192) NOT NULL DEFAULT '',
 finalization_grace_millis BIGINT NOT NULL DEFAULT 0,
 terminal_rule VARCHAR(160) NOT NULL DEFAULT '',
 draw_prestige INT NOT NULL DEFAULT 0,

  `group` CHAR(1) NOT NULL,
  `enabled` BOOLEAN NOT NULL DEFAULT 0,
  `version` CHAR(64) NOT NULL,
  `source` TEXT NOT NULL,
  `participation_rewards` TEXT NOT NULL,
  `victory_rewards` TEXT NOT NULL,
  `transport_mvp_rewards` VARCHAR(2048) NOT NULL DEFAULT '',
  `personal_missions` VARCHAR(4096) NOT NULL DEFAULT '',
  `task_rewards` VARCHAR(2048) NOT NULL DEFAULT '',
  `factory_mission_prestige` INT NOT NULL DEFAULT 0,
  `win_prestige` INT NOT NULL,
  `lose_prestige` INT NOT NULL,
  `published_at` BIGINT NOT NULL,
  PRIMARY KEY (`group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
