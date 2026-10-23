# 萌新福利原UI guard

仅给原 `UI.ShowRight::<OnTenDaySignClick>m__154` 与 `UI.TenDaySignPage::UpdateLoginReward` 插入条件前缀。原非空列表的所有指令与领取路径保持，服务端记录、奖励和回执不改。关闭/全领不再打开默认占位页；直接收到空列表会清理旧grid、隐藏prefab，经原Page.Close生命周期关闭并使用原Box提示。真实已领取的非空列表仍可由原Update读取。

构建必须提供固定APK与SHA，输出新overlay目录，`--tool-root`指向已有只读dotnet/dnlib工具链。Python需要项目 `.analysis-venv/bin/python` 的dnfile/dncil。构建验证547原引擎和所有非目标raw body不变、两guard实际消费与合法分支原指令/跳转不变。输出main+HYJS.NewbieSign.dll和manifest，不打包、不安装、不操作手机。

`verify_guard.py`运行实际helper源的8个分支案例，UI使用最小double，仅作逻辑验证；原Unity引用C#3编译与IL消费验证由build完成。原机画面与完整APK由主代理合批。

## 仅QA诊断覆盖层

`build.py --qa-diagnostics` 严格固定55正式APK SHA `b7ad3c7c8ba53cc7990f3f3590642aa405e9a554135916a468668f0c7fdbdbbd`，要求输入已含两处正式guard，不重复插入。使用 `/define:HYJS_NEWBIE_QA` 替换helper并增加RoomTown自动打开来源及原Update每个return出口日志。正式默认构建不定义此符号、不添加诊断钩子。

日志使用独立Android tag `HYJS.NewbieSignQA`，绕过被抑制的Unity Debug。仅记录匿名success/isShow/itemsCount、page/grid/childs/prefab结构与active状态、decision、render-exit，不含角色、会话、资源ID或奖励内容；日志异常被吞，返回行为不变。若有render-enter/decision=true而无render-exit，可定位到原渲染体中途失败；auto日志解释自动登录入口绕过CanOpen的路径。

当前小overlay：`analysis/dungeon56-20261023/newbie-qa-diagnostics`。此覆盖层仅可用于诊断QA包，禁止提升正式current。验证剥除诊断指令后原体指令/分支相同，其他26944个raw方法及547个引擎方法保持。原Unity实际渲染仍须由QA日志和画面确认。
