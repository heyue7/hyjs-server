# HYJS 项目工作指引

## 入口与职责

- 研发、续接、修复、验证、发布和进度核查使用 [hyjs-development](skills/hyjs-development/SKILL.md)。用户只要求分析、方案或规则整理时，保持对应范围。
- 当前范围、优先级、业务规则和进度只维护在 [plans](plans/README.md)。只读取与本次变更有关的计划；稳定规范不记录玩法、批次和现场数据。
- 主代理先查源码并负责技术方案、接口、表设计、锁序和验收，再审查、集成。SubAgent 只执行明确的文件/命令/实现步骤，超出方案返回主代理；单项小改直接完成。
- 委派使用 Codex 原生 `collaboration.spawn_agent`，显式 `model="gpt-6.1-terra"`、`reasoning_effort="max"`、`fork_turns="none"`；任务自包含，约定文件与资源所有权。不通过其他 CLI 委派，不擅自替换模型。
- 服务端遵守 [hyjs-server/AGENTS.md](hyjs-server/AGENTS.md)。设计按实际数据归属和读写边界划分，个人操作不借用公共写门；共享资源只保护实际竞争的最小键。

## 工作环境与授权

- Shell 按 `/Users/lirui/.codex/RTK.md` 使用 `rtk` 前缀。禁用“不是……而是……”句式。
- 浏览器操作前读 `/Users/lirui/.codex/BROWSER_RULES.md` 和所选 skill；使用 hybrid gateway，机械流程按其要求委派 `routine`。
- 本机 DB/Redis/Docker、启动或数据库验证前读 `/Users/lirui/.codex/local-services/README.md`；复用共享实例、项目分配与资源租约，保留远端契约。
- 未经单独明确授权，不发布对外通知、批量通信或生产数据补写；产生资产变化的验证使用隔离环境。具体运行合同按变更领域查 [运行与验收约定](plans/runtime-contracts.md)。
- 使用隔离工作树保护既有混合改动，同文件一个写入者；凭据不打印、不提交。默认不增加基础设施或扩大机器限额来替代代码优化。

## 资源与交付

- 及时清理归属明确、无运行或停止引用且可再生的自有资源；保护共享服务、源码、凭据、业务数据、必要备份和当前制品。禁止全局 prune/flush，存疑数据保留并说明。
- 先减少重复工作和写放大，再处理有生命周期依据的无引用数据；清理与迁移有界、可中断恢复，不放进普通请求链。
- 发布及持久化变更只按已授权范围执行，核实际版本、运行状态和恢复路径。时限包含构建、关键验证、清理和汇报。
