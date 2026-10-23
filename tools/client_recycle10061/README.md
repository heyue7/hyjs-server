# 回收屋100批量客户端补丁

固定输入为59 APK SHA256 `957ca6dd731e5d87d1d103f6fe1c76081f8491dc331581437eaffcdf3cdd265b`，Main SHA256 `d4fefebc5239c16421b7bd57418e2106f4f28ca6905bff83400fd338c1dfb6b2`。构建器只输出managed overlay，不合包、安装、部署或更新正式指针。

## 已实现

- `StaticData.recycleMax` 保持原40，仅分解专用数组与人物/装备/精灵选择缓存改100；33个原Main方法按固定token及CIL SHA定点修改，重置页与原定义/字段RID保持。
- 主面板4个图标保留原循环，中心增加 `已选 N/100` 和查看提示。选择页在原确认按钮右侧显示N/100与预查状态，字体继承原UILabel；没有新增素材或改按钮造型。
- 三个一键事件继续调用原公开 `HYJS.Recycle.SelectionControl.Select(RecycleSelectPanel)`；资格接口limit100，整组校验可见候选/重复/空值/容量后提交。旧完整选择保留到成功新选择；失败、读超时保持旧值；off清空100，generation隔离旧响应。
- 原主面板添加装备/角色/精灵入口打开对应原选择页并自动执行相同预查。打开后只选择，用户确认后回主面板再点分解。
- 手选、取消、缓存复制、重开均使用100容量。品质>=20警告扫描全部100，原警告文案和三个确认分支保留。
- 分解一次构建一个原请求：equipment `CSEquipDecompose.guid/21009`、hero `CSWarriorDecompose.guid/21021`、pet `CSPetDecompose.guids/21517`。所有选中GUID进入同一个原protobuf消息。
- 未知网络结果保留immutable GUID快照和pending，不用20秒后另发新stamp。沿用原HTTPManager超时框与原序列化字节重试。明确ErrorMessage失败才释放pending，原选择保留；成功仅一次调用原成本/奖励回调。
- 原成功回调、原奖励协程、原奖励页和原HTTP传输/重试方法全部raw body精确保留。页面关闭时未决请求保留选择；相同会话下迟到成功，在关闭页面时使用相同原数据函数应用，不启动不可见页面动画。登录会话变化会使旧回调失效，由登录重新取得权威资产。

## 入口和生命周期

`Injector.cs`逐目标锁定输入SHA并保留原locals签名，目标均无EH。`Flow.Submit`包装请求及回调，`Flow.ClearGlobal`在普通关闭时清空100，在未决请求时恢复其完整快照。`AllowSelection/AllowPanel`阻止pending期间手改、切类型与再次确认；原Http timeout重试入口可继续使用。

当前一键仍沿用59 helper同时写本页cache及全局选择的行为；选择页X返回会保留已自动选，off清空后可确认空选择。手选仍按原确认复制cache。若主代理另行统一“选择页确认才提交”的语义，需要同步补入口快照与X取消恢复，不能只改一处写入。

## 构建与验证

```sh
rtk proxy .analysis-venv/bin/python tools/client_recycle10061/build.py --base-apk build/hyjs-feedback59-r4.apk --output-directory build/recycle61-formal-r3
```

默认base也是固定 `ROOT/build/hyjs-feedback59-r4.apk`，不跟随 `hyjs-local.apk` 当前发布指针。base必须仍匹配本文顶部59 SHA。输出目录必须不存在。编译在本轮临时目录完成并自动清理。manifest记录实际sourcePins、原引用SHA、目标token、每个目标locals/EH核对及overlay各项SHA。

构建器调用59已验full-body门禁：全部非目标方法header/CIL/locals/EH字节精确不变，547个原引擎方法精确不变，FieldPtr/MethodPtr缺席，Main无新字段/类型/方法。额外验证全局recycleMax40、专用数组100、三处原4预览循环、原成功回调/动画/重试/奖励页body不变。

`Probe.cs`运行实际已编译helper：0/1/4/5/40/99/100选择，101/重复/隐藏/空GUID/旧40容量拒绝，手取消后99、完整100快照，三种真实原protobuf对象各含100GUID，以及callback owner/once条件。probe不执行Unity、网络、MySQL或手机。

## 当前原机证据与r3定位修复

主代理已在r2原机确认主面板中心计数良好；选择页计数实际落在第一行材料文字中间，截图为 `analysis/recycle61-20261024/phone61-equipment-auto100-preview.jpg`。r3只修选择页计数定位，主面板分支、100选择/请求/交易代码保持。

根因：原 `Main/ConfirmBtn` 的文字是子节点 `Sprite`，其下没有UILabel；r2寻找按钮文字失败后取了第一行材料UILabel作为锚点。`layout-evidence.json` 保存固定59 prefab原层级、坐标与控件尺寸。三类面板都使用 `Main/ConfirmBtn` 中心(86,-228)、128×70，`Main/Di` 为TopLeft(-313,268)、780×540，右边界467。字体明确取 `Main/OneKeyChose/Label`；新计数与ConfirmBtn同父 `Main`，X按运行时按钮右边界到Di右边界之间实际空白区居中，Y/Z跟随ConfirmBtn，继承原footer TweenPosition/Alpha，不依赖材料行或猜测固定偏移。r3布局尚待主代理原机复核。

## 原机待验

- 使用三个类型各>=101可选且含保护对象的隔离账号，核第100项确实勾选、取消/补选、第101项限制、实际请求与cost及资产减少均为100。
- 主面板N/100中心计数在r2已通过；复核r3选择页确认按钮右侧计数同视口不遮挡，原字体、滚动及触控可用。r3新定位尚未原机验证。
- 一键on/off/on、旧响应、预查失败/超时、空列表、主添加入口、手选、高品质对象位于第5/100项、取消警告、关页重开、类型保存。
- 原网络超时重试/关闭提示/重登录、提交后丢响应、业务失败、重复成功回包；确认100选择不丢、原字节重试不二次结算/本地加奖；冷重登录核资产。

服务端100限额、原子事务、幂等、保护与实际DB验收由主代理负责合批。此目录的编译和probe通过只代表对应静态/纯策略范围。
