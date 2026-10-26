# Batch73 managed overlay

基线正式72 SHA固定在build.py，工具仅提取并输出两个managed DLL与manifest，不打包不连接服务。

`/Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_sprout73/build.py --base-apk /Users/lirui/LR/hyjs/build/hyjs-1.0.72.apk --output-directory analysis/growth73/client/overlay-v4`

打包按manifest.payloadFiles替换原Assembly-CSharp.dll并增加HYJS.Sprout73.dll，其他APK entries继承基线。三个hook为Room.RCUIPlayer.DoStart尾部Attach、DoStop头部Detach、Dict.Dao.TableCopyExpDao.ParseRowData返回行dup/Experience。经验绝对映射100行，重载不会乘叠，coin保持原值。

小芽使用原PNG alpha裁边，图标与原TextMesh同父变换，沿原材质/层级，visible height=字高，gap=0.15字高；原姓名等位置保持。自身/其他玩家仅RoomCharactor type 1/2，owner.name已由原CreateCharactor IL核为ID。网络认证/原点沿GGOShop；同场显示名单变更或30秒刷新，15秒超时，校验当前角色/session/origin与返回ID归属，到期按服务器偏移隐藏，未知隐藏。角色/区服变化清缓存；DoStop清空overlay后归还原pooled label，保留空组件供pool复用，避免同帧复用命中延迟销毁。quad仅字形bounds变动时重建。

静态证明仅覆盖编译、547原引擎及其他方法字节、100行公式。实际手机布局、遮挡、字号/镜头、服务返回、生效/到期/换号/退场仍待主代理验证。
