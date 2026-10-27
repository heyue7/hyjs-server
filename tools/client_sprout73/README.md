# Batch73 managed overlay

基线正式72 SHA固定在build.py，工具仅提取并输出两个managed DLL与manifest，不打包不连接服务。

`/Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_sprout73/build.py --base-apk /Users/lirui/LR/hyjs/build/hyjs-1.0.72.apk --output-directory analysis/growth73/client/overlay-v4`

打包按manifest.payloadFiles替换原Assembly-CSharp.dll并增加HYJS.Sprout73.dll，其他APK entries继承基线。三个hook为Room.RCUIPlayer.DoStart尾部Attach、DoStop头部Detach、Dict.Dao.TableCopyExpDao.ParseRowData返回行dup/Experience。经验绝对映射100行，重载不会乘叠，coin保持原值。

小芽使用原PNG alpha裁边，图标与原TextMesh同父变换，沿原材质/层级，visible height=字高，gap=0.15字高；原姓名等位置保持。自身/其他玩家仅RoomCharactor type 1/2，owner.name已由原CreateCharactor IL核为ID。网络认证/原点沿GGOShop；同场显示名单变更或30秒刷新，15秒超时，校验当前角色/session/origin与返回ID归属，到期按服务器偏移隐藏，未知隐藏。角色/区服变化清缓存；DoStop清空overlay后归还原pooled label，保留空组件供pool复用，避免同帧复用命中延迟销毁。quad仅字形bounds变动时重建。

静态证明仅覆盖编译、547原引擎及其他方法字节、100行公式。实际手机布局、遮挡、字号/镜头、服务返回、生效/到期/换号/退场仍待主代理验证。

## 77 候选小芽显示修复

正式77候选 `c970012d7c785c58c3b923ea4a0ea8e650204ed0e50e16e914f47e54ac4aece8` 已携带三个原有 hook 与 `HYJS.Sprout73.dll`。姓名 prefab 的 TextMesh 使用 MeshRenderer，缺少 MeshFilter；运行时改取原 Renderer.bounds 并按文本变换恢复局部宽高，图标复用原称号的彩色透明材质和文字排序。该图标独立于姓名和称号，沿原位置显示。

可见名单最多100人，优先包含可见本人；名单变化立即作废在途旧结果，离场时释放对应缓存。原同场鉴权和整批拒绝保持，批次失败时每个原查询最多追加一次本人查询，后续仍按既有30秒周期或名单变化刷新。换号、换区、过期和无资格状态清除或隐藏图标。

轻量检查执行 `rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_sprout73/verify_logic.py`；这使用候选 APK 自带 LitJson，以及 Unity 和网络替身，不能证明实际 Unity 渲染。编译现有候选的单程序集覆盖：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_sprout73/build_refresh.py \
  --base-apk build/activity-growth77/hyjs-1.0.77-candidate-r8.apk \
  --output-directory build/feedback78/sprout
```

脚本校验固定 APK SHA、三个现有 hook、helper 公开 API 和 547 个原引擎方法库存，manifest 仅替换该 helper；没有重新打包或安装。实际小芽资格、同场切换以及文字和图标视觉仍需在运行环境验证。
