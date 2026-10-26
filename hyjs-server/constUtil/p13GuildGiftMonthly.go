package constUtil

import "time"

const (
	P13GuildGiftRule        = "local-original-guild-gift-calendar-month-v1"
	P13GuildGiftSource      = "original-UnionConfig-level1..10-role0..2-reward-center;verified-real-daily-login-only;AsiaShanghai-[monthStart,nextStart);legacy-archive-no-retro"
	P13GuildGiftStartsMonth = "2026-11"
	P13GuildGiftBatch       = 128
	P13GuildGiftMaxMembers  = 62
	P13GuildGiftInterval    = time.Minute
	P13GuildGiftConflict    = "公会月礼包认证来源或周期状态损坏"
	P13GuildGiftAdminActive = "本月活跃由真实登录认证累计，不能直接修改"
)
