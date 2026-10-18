-- 本地服解释：APK odds归一为相对权重，每箱独立抽一次，原服概率分母未恢复。
INSERT INTO treasure_box_rules(item_id,enabled,mode,max_batch,source) VALUES
('item_022',1,'normalized_weight',100,'APK TableItemUse reward+odds; local normalized single draw, original denominator unknown'),
('item_024',1,'normalized_weight',100,'APK TableItemUse reward+odds; local normalized single draw, original denominator unknown'),
('item_025',1,'normalized_weight',100,'APK TableItemUse reward+odds; local normalized single draw, original denominator unknown'),
('item_115',1,'normalized_weight',100,'APK TableItemUse reward+odds; local normalized single draw, original denominator unknown'),
('item_116',1,'normalized_weight',100,'APK TableItemUse reward+odds; local normalized single draw, original denominator unknown'),
('item_117',1,'normalized_weight',100,'APK TableItemUse reward+odds; local normalized single draw, original denominator unknown');
