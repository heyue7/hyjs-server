# 雪原两点r2交付

根因及原创建IL证据见field-snow-two-proposal.md：原导航首次100m投影导致两点合法初始位置超出服务器出生锚点R2，现有路径首corner持续触发path-range。

已按Root批准，仅为field_002、anchorXZ=(66,62)/(62,49)、配置R2提供固定VisualAnchor兼容。须source超原R2、原navigation first=false且idle、该绑定epoch PatrolStarts=0，并且只读SamplePosition(authoritativeAnchor,100,-1)与source三维距离<=0.1m，才绑定原合法投影出生位置。其他点VisualAnchor保持原anchor。

uuid/monId/权威anchor或纠偏检测均保持。身份/monId/权威纠偏变化会失效视觉中心并重置本地诊断epoch计数；正常同锚push、暂停恢复、失败和巡游不会重新选中心。现有纠偏仍走原StopTo。所有目标及完整path corners仍在固定视觉R2内，源偏差/2m层高/完整路径/8次每秒/原speed和StopTo门禁保留；无位置赋值、warp、直线fallback或碰撞修改。VisualAnchor/VisualAnchorStatus是本地只读诊断字段。

单次构建输出独立build/field64-overlay-r2，没有覆盖r1。C# helper/injector无警告错误；r2 Main与r1 Main SHA完全一致，只有helper内容变化。相对正式63只新增Boss Tick，547原引擎、26945其他raw方法及metadata ABI/tokens仍精确。详见field-snow-two-r2-proof.json。

未操作手机/DB、未打包APK。Root继续原机检查两点Starts/Arrivals与实际位移、墙体/层高限制，并复核49只和Boss。本任务无自有后台，临时构建目录自动释放；r1当前引用与r2待验交付均保留。
