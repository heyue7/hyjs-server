package constUtil

import "time"

const (
	P12FieldPVPMaxHeroes            = 5
	P12FieldPVPPresentationInterval = 250 * time.Millisecond
	P12FieldPVPPresentationBatch    = 32
	P12FieldPVPAttackingState       = int32(6)
	P12FieldPVPDefendingState       = int32(7)
	P12FieldPVPPolicyName           = "local-field002-pk-transaction-v1"
	P12FieldPVPSource               = "original30045/type5/max40/factors1/3s-countdown+3s-play; local10m/current-TCP/same-line/non-safe; atomic-server-terminal; ordinary-white-target-2percent-coin-cap; judicial-send-keeps-jailFlag; ordinary-red-death-minus1; original-town-prison-after-busy; jail=max(TableKiller-minutes,kills*180s); paid-first10000-then-kills8334; defaultoff"
	P12FieldPVPRecordSource         = "p12-field-pvp"
	P12FieldPVPMapClosed            = "当前区服的该野外地图未发布PK许可或已关闭"
	P12FieldPVPClosed               = "当前野外PK本地策略未开放"
	P12FieldPVPBusy                 = "当前野外PK演出尚未结束"
	P12FieldPVPJailed               = "当前角色仍在监狱刑期内"
	P12FieldPVPConflict             = "野外PK冻结回执校验失败"
	P12FieldPVPBusyMS               = int64(6000)
	P12FieldPVPPrisonX              = float64(58.22999954223633)
	P12FieldPVPPrisonY              = float64(-2)
	P12FieldPVPPrisonZ              = float64(10.699999809265137)
	P12FieldPVPRadius               = float64(10)
)
