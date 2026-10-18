package constUtil

const (
	P8SkillBookOperation     = "life-skill-book"
	P8ShortcutOperation      = "field-shortcut"
	P8HpRecoverOperation     = "field-hp-recover"
	P8ResurrectOperation     = "field-resurrect"
	P8SafeReviveOperation    = "field-safe-revive"
	P8FieldDead              = int32(5)
	P8FieldBattle            = int32(6)
	P8FreeReviveWaitMS       = int64(10000)
	P8SafeReviveProtectionMS = int64(3000)
	P8LifeSkillSource        = "APK TableItem(category15)+TableItemUse技能ID；SCItemUse后原UI查询SCCookingLevel.skills"
	P8LifeStateSource        = "APK FieldBuyLife/InvincibleTime/Resurrection2Safe/HpRecoveryPercent/FieldHpCD和UIFieldAskEaster10秒计时；有限单人服同步百分比血量，原地复活怒气清零，旧死亡档从首次恢复计时"
	P8EventLifeBook          = "life_skill_book"
	P8EventHpRecover         = "field_hp_recover"
	P8EventResurrect         = "field_resurrect"
	P8VisibleHpUnavailable   = "当前无法查看该玩家的野外血量"
)
