-- APK恢复觉醒费用、英雄70级门槛、两阶段、融合经验与100级上限。
-- SQL仅补充未恢复的服务端金币费用与材料自动选择策略。
CREATE TABLE IF NOT EXISTS warrior_awake_rules (
 id VARCHAR(32) NOT NULL PRIMARY KEY,
 enabled BOOLEAN NOT NULL,
 coin_cost BIGINT NOT NULL,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
