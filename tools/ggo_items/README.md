# GGO 道具与原生兑换所

六品质芯片 `item_9701`～`item_9706`、回收信用点 `item_9710`、红品质超频点数 `item_9711`。角色头顶携带标识与小芽另提供64px/24px透明素材。生成原图、提示词和原品质框来源保存在本目录，包内图标使用原游戏 AssetBundle/TextureKit 加载路径。

## 构建顺序

固定旧正式包1.0.51 SHA-256：`464297015905ee0d5a0194b8cbfaf623d8b32111c972c5ce150c21e538821856`。提升52后，固定输入保留在原主目录 `build/hyjs-formal51-rollback.apk`；`hyjs-local.apk` 为当前交付指针。

1. `build_assets.py --base-apk <固定51包> --output <新资产目录>`：生成10个独立CAB身份图标、原生道具表及文本、两份资源索引，同时写出 `client/GGOIconManifest.cs`。
2. `build_client_overlay.py --base-apk <同一51包> --output <新客户端目录>`：编译原商店组件扩展与生成清单，仅插入3处UI钩子，校验原547战斗方法与其他原方法。
3. `package.py --base-apk <同一51包> --assets <资产目录> --client <客户端目录> --output <build内新APK> --record <analysis内新记录> --build-code 2026102304 --version-name 1.0.52`：核对生成清单来源与资产哈希、签名和对齐、继承条目压缩字节。

命令由 `rtk proxy .analysis-venv/bin/python` 执行。使用全新的输出目录与记录；生成清单与资产必须同次构建。签名沿用项目私密现有签名文件。

## 旧安装兼容

原加载器优先读取玩家已有 `smallassetmanifest/assetmanifest`。`GGOShop.RegisterIcons` 通过原生解析器将生成清单的10个缺失键加入运行时字典，保留原索引项与磁盘文件。每个新图标包有独立bundle名/CAB，避免模板重名；原生泛型签名引用原主模块TypeDef。

## 业务边界

购买使用角色会话、版本、持久UUID回执及数据库事务。超频点数获得后在原存档事务内转为账号终端余额；后台单项、编辑和批量库存操作在原差量保存前执行同一转换。1道具=1完整点数=100终端最小单位。

沙漠的NPC、提示、小怪/Boss及复活时间保存在后台设计配置。坐标、芯片结算值及搜打撤运行机制留待后续玩法实施。
