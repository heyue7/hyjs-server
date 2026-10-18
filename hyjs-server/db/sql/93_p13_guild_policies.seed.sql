-- 默认关闭管理与经济门禁；不覆盖既有运营状态。
INSERT IGNORE INTO p13_guild_policies(id,management_enabled,economy_enabled,chat_enabled,source) VALUES (1,0,0,0,'原TableUnionConfig/GameConfigure/UnionLogo与原UI IL；兼容策略和待审计机制见analysis/p13/guild-recovery.md');
