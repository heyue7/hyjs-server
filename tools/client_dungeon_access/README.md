# Scoped dungeon content adapter

`Runtime.cs` compiles as **HYJS.DungeonContent.dll**, public static class `HYJS.DungeonContent.DungeonContent`. Target the original Unity/.NET 3.5 references and fixed current55 Assembly-CSharp (SHA `c575957a4d2615be7d25cc926d2dea1b8910925ee5f8b1b11880517f0b6c6d19`), C# 3, `/nostdlib+`. No runtime JSON library is required. `catalogue.json` is build-time policy; runtime contains the same 21 explicit main-story aliases and 36 scene names.

## Injector integration

Compile `Injector.cs` as a net8 console using the existing dnlib reference. Invoke with exactly `inputMain helper outputMain`. Helper assembly identity must be `HYJS.DungeonContent`; import helper methods with dnlib against this exact compiled helper. PreserveAll and KeepOldMaxStack are set; each changed body reserves at least eight stack slots. The injector adds no locals and does not simplify or optimize original branches.

`targets.json` supplies the exact eight input CIL SHA pins, method tokens/signature blobs, helper signatures and anchors. The injector independently checks those original raw CIL pins and rejects already installed helper calls. It writes `changed-methods.json` next to outputMain. Unrelated earlier/later overlays must not change these eight original bodies. The build owner must verify input APK identity, all non-target CIL/metadata, 547 original Engine methods, resources, helper imports and final package composition.

Public helper entry points:

- `string Scene(string original)`: existing physical/manifest resource remains original; only explicit missing catalogue names with available whitelisted target get an alias. Unknown names return unchanged and original GetReplaceSceneName logic runs.
- `bool MainEntryReady()`: selected current UICopy mission must have canonical ID 1..161, existing table and tasks, and scene aliases in the verified whitelist with available map bundles. Native chapter <=10, bags, stamina, challenges/reset and CSCopyEnter remain in the original method.
- `void AdaptMission(Room.RoomCopy room)`: after native server missionScene override and before mini-map setup; records original/replacement and synchronizes task/jobs scene strings. Same room/mission resume preserves prepared positions and applies them to refreshed job spatial fields.
- `void Prepare(Room.RoomCopy room)`: first-load branch only; uses loaded NavMesh connected points and encounter spacing; writes mission/task/job BornX/BornZ only. No CameraAngle, FaceTo, AI radii, BornType or combat fields are modified.
- `void AlignPlayer(Room.RoomCopy room)` and `void AlignMonster(Room.RCAIMonster ai)`: scoped transform/NavMeshAgent alignment, with original task IDs. Only the single TableCopyTask Init overload is hooked.
- `Vector3 ProjectTarget(Vector3 original)`: only prepared substituted copy scene3; original target with SamplePosition within 0.05 remains unchanged, otherwise native NavMesh projection up to 100 units.
- `bool SkipArea(string area)`: only exact mission_<current substituted mission>, after preparation. Other room areas retain native behavior.

## Layout source and limits

Source evidence is `analysis/dungeon56-20261023/resource-mapping.json`, recovered from RE OfflineBootstrap SHA `b26c6b453e5a534bcdfa7da5d302f9266c9ade88e813129d6ecd321ebe59179b`. Triangles are sampled with barycentric eighths, NavMesh.SamplePosition radius1, rounded half-unit x/z deduplication, z/x ordering, at most 12 anchor candidates, largest fully reachable sample set, and greedy farthest encounters with minimum squared spacing4 (two units). Every task point must have complete path from spawn. An insufficient/absent NavMesh throws a diagnostic error; arbitrary fallback coordinates are not generated. Prepared layout is scoped to current room+mission and retained across its battle return.

Missing original scenes use existing substitute scenes; original scene layout is not restored. Skipping old mission spatial areas means original area-based story presentation is unavailable in those layouts. Native story event calls and original LoadWall remain intact: native wall loading disables numeric donor blockers and only enables matching TableWall.MapId configuration. No new wall contract is introduced. Native task IDs, monsters, skills, random seeds, requests, engine and settlement/rewards remain unchanged. Music still uses the original BattleData scene string on battle entrance; this adapter does not claim missing original audio is restored.

No global `full`/`downover` writes, download completion or CSBigClientReward changes. No awakening field layout adapter, offline battle entry or offline sweep/reward behavior is imported. Model compatibility remains the separately existing HeroArtFallback responsibility; scene substitution alone does not prove all art dependencies.

## Local validation

Independent self-check compiled the helper with original Unity references/C#3 and the injector with net8 dnlib; injected all eight fixed55 methods. `analysis/dungeon56-20261023/runtime-injector-raw-exact-selfcheck.json` records exactly eight changed raw CIL bodies and **26,938** other raw CIL bodies unchanged, including the other RCAIMonster.Init overload. Original Engine 547 instruction bytes/EH validation also passed. These are managed/static checks. Android execution and actual loaded-NavMesh encounter traversal remain required integration checks owned by the parent task.

## 觉醒独立适配

现目标为12方法，保留原8个主线hook，新增RoomChallenge.StartLoad/OnLoadComplete/LoadPlayer与FallItem.OnGoSailClick中唯一觉醒full表达式。sceneType10独立于主线type3，只有实际替换导航图时重排；觉醒原图/未替换图保持坐标。资源白名单37图，含父代理无损补齐的nightforestfield001原路径；不为它增加别名。4个新增alias严格来自RE OfflineAwakeCopyAssets310.Register：ice001→snowforest001、volcano001→cave001、volcanofield001→forestfield001、desertfield001→desertrun002。

AwakeAdapt只映射已确认chapter.sceneId/BattleMap，不动chapter/monId/heroId。AwakePrepare在原首次LoadPlayer前读取当前真实SCEnterAwakeCopy，按原ViewR+wanderR邻接识别遭遇分组，组内只平移至新图NavMesh并验证邻接不拆、组间不合、出生安全和每个对象路径。宝箱保持原怪物保护关系，无法容纳明确拒绝；不缩AI半径、不跳过对象。通过全部验证后才更新客户端可视bornPos/monsters.pos/boxes.pos，服务端冻结、奖励、任务与战斗引擎不改。AwakeAlignPlayer补NavMesh真实Y；已有载入房间/同入场对象返回保持同布局。

Layout复用NavMeshPath；仅两方向PathComplete确认同强连通分量时复用其reachable集合，保留最终每个实际遭遇的路径验证。不会把单向OffMeshLink误判同连通域。

Injector保留TryToUseTypeDefs，且第一次写出后重载metadata，将所有错误指向ModuleRef HYJS.DungeonContent.dll而实际属于Main的TypeRef重新绑到Main；最终再读字节强验残留0。证据helper-scope-normalization.json，不能以写前GetTypeRefs枚举0代替最终字节验证。

本次C#3 helper、Injector实际编译/注入及12targetSHA核验通过；547原引擎方法与26934非目标raw保持，最终helper错误scope0。证据analysis/dungeon56-20261023/awake-selfcheck；37场景完整闭包及原机NavMesh空间验收由父代理合批，编译成功不代表所有地图布局已经实跑。缺怪donor与原非model缺项另批，未修改原outside.heroId或032兜底策略。
