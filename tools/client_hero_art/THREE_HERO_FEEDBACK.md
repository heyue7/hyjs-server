# 三角色图鉴修复

只将凪069、罗亚070、克洛维073缺失的头像与hero2d路径映射至现成原画hero_card；原AssetLoad及TextureKit负责加载，widget负责单次裁切。普通图鉴、角色详情与原招募预览点击链共用此边界，其他角色路径保持。

hero_copy.json为三人简介、技能名称/描述和30行系数的唯一编辑源。文案为本轮原创，模型武器/动作/世界取自原包；网上没有找到可与资源身份绑定的官方人物简介，不能当成已恢复正传设定。

Root复核倍率采用实际橙卡hero010，前排单体物理普通技总量100，必杀620、原每级成长12。保留原技能段数、动作时间、选择策略与状态字段。gen_hero_copy.py生成HeroCopyTable.g.cs；--server同时生成Go启动投影。原TSV不改。build_skill_factors.py仅在原TableSkillCastDao.ParseRowData返回前追加校正，其他原方法及547战斗方法保持。verify_native_factors.py执行原DAO并核30目标行和1对照行，不声称执行引擎战斗。

原机和发布状态见plans/27-play-feedback-20261020.md。
