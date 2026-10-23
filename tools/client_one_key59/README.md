# 原生出售与回收选择59

复用原 BagSellPage 品质勾选、物品标记、价格汇总、最终二次确认和出售协议。人物/装备/宝物/精灵及原碎片类型调用原完整出售候选接口，保留上阵、穿戴、锁定、训练和原品质条件。候选按原接口顺序选取；每次最多100件，包含已有选择；达到上限会明确提示剩余可在出售后继续，不自动分批发请求。手动添加也遵守100件，取消不受此上限阻止。

六个原方法钩子见 targets.json。品质按钮排队到下一帧读取原 UIToggle 最终值；取消按候选GUID移除，覆盖其他页并避免索引已销毁图标。原分页协程各返回点仅调用 Draw 同步当前可见标记；原返回bool、局部变量、控制流和异常区域保持。原 DoOpen 仍清空选择、重置品质勾选。

SellCallback仅在原bool成功分支进入后调用BeforeSuccessfulRefresh，停止该BagSellPage旧分页协程并补curPetCount=0。原库存/收益/父回调/选择与勾选清理以及唯一列表刷新仍由原成功body执行，原前三计数重置不重复。false分支直接跳原ret，不调用helper，不动失败/超时/重试或实际出售协议。原刷新含0.5秒空窗，截图瞬态不等同永久分页错误。

先修复旧 InstanceProtection 的三处错误 TypeRef scope（UIHero、UIEquipment、UITreasure 从 InstanceProtection.dll ModuleRef 归还 Main）；metadata-only 中间件全部原方法 header/CIL/locals/EH 字节不变。无字段、方法、类型定义新增，无 FieldPtr/MethodPtr。未来应在原 instance_lock51 Injector 做相同导入归属修正，打包时对所有 helper-scoped 原Main类型引用加门禁；不要删除原生ModuleRef。

现有 tools/client_recycle_selection/Selection.cs 也重新编译为 SoulFlowerRecycle.dll。服务器返回GUID必须完整验证：唯一、数量不超过原4槽/上限、槽数组容量有效，并同时存在于原合法候选和当前GUIItem映射。全部通过后写原cache/slot/标记；任何失败清空并提示。未添加自动分解/自动出售。

构建（仅managed，不合包/安装）：

```
.analysis-venv/bin/python tools/client_one_key59/build.py --input-managed /absolute/formal58/Assembly-CSharp.dll --output-directory build/onekey59-formal-r3
```

固定55 APK提供原C#3/Unity引用，输入Main支持串行继承。manifest记录原SHA、sourcepins、metadata-only全部原方法精确、最终仅6目标变化、547引擎和非目标完整原body精确。分页协程校验去除返回点Draw后原指令/跳转/EH/locals语义完全相同。实际helper纯逻辑回归覆盖全候选320、含已有选择的100上限、跨页取消、重复不占新槽，以及回收隐藏GUID/重复/超限/空GUID/坏槽拒绝；不将此称为原机UI验收。
