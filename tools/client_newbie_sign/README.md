# 萌新福利原UI guard

仅给原 `UI.ShowRight::<OnTenDaySignClick>m__154` 与 `UI.TenDaySignPage::UpdateLoginReward` 插入条件前缀。原非空列表的所有指令与领取路径保持，服务端记录、奖励和回执不改。关闭/全领不再打开默认占位页；直接收到空列表会清理旧grid、隐藏prefab，经原Page.Close生命周期关闭并使用原Box提示。真实已领取的非空列表仍可由原Update读取。

构建必须提供固定APK与SHA，输出新overlay目录，`--tool-root`指向已有只读dotnet/dnlib工具链。Python需要项目 `.analysis-venv/bin/python` 的dnfile/dncil。构建验证547原引擎和所有非目标raw body不变、两guard实际消费与合法分支原指令/跳转不变。输出main+HYJS.NewbieSign.dll和manifest，不打包、不安装、不操作手机。

`verify_guard.py`运行实际helper源的8个分支案例，UI使用最小double，仅作逻辑验证；原Unity引用C#3编译与IL消费验证由build完成。原机画面与完整APK由主代理合批。
