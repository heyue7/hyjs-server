package constUtil

type LocalPlayContextKey string

const LocalPlayBoundKey LocalPlayContextKey = "local_play_bound"
const LocalPlayOwnerKey LocalPlayContextKey = "local_play_owner"
const LocalPlayOffsetDaysKey LocalPlayContextKey = "local_play_offset_days"
const LocalPlayVersion = "local-single-player-tools-v1"
const LocalPlaySource = "用户授权的本地单人游玩工具v1：仅dev/test显式钱包启动及demo角色，排除历史账号1；个人游玩日期可前后调整和恢复，原支付账单及网络会话使用真实时间；活动新期独立保存，历史回执不清空。默认关闭。"
const LocalPlayDisabledMessage = "游玩工具暂未开放"
const LocalPlayInvalidMessage = "游玩工具请求或当前会话无效"
const LocalPlayConflictMessage = "本次操作编号已用于其他操作"
const LocalPlayBusyMessage = "请先结束当前战斗再调整游玩日期"
const LocalPlayRangeMessage = "游玩日期最多前后调整3650天"
