-- base_only 是显式本地服规则：仅返还 APK TableEquipDecompose.retRes。
-- 原服强化、精炼、升星和锻造投入返还公式未知，当前不追加材料。
-- 可改成 reject_progressed，拒绝 level>1/refine/star/forge 的装备，要求先重置。
INSERT IGNORE INTO equipment_recycle_rules (id, enabled, progression_refund, source)
VALUES (1, TRUE, 'base_only', 'APK TableEquipDecompose.retRes; local base_only: original progression refunds unknown');
