package service

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// P13UFFunctionalRulesSummary is an internal publication/audit result, not a
// replacement for enrollment, a frozen match, a combat capability or rewards.
type P13UFFunctionalRulesSummary struct {
	Day                string                  `json:"day"`
	PublishedAt        int64                   `json:"publishedAt"`
	DayCreated         int                     `json:"dayCreated"`
	DayReused          int                     `json:"dayReused"`
	GlobalsChanged     int                     `json:"globalsChanged"`
	DayContentHash     string                  `json:"dayContentHash"`
	GlobalContentHash  string                  `json:"globalContentHash"`
	OriginalTablesHash string                  `json:"originalTablesHash"`
	GuardProof         string                  `json:"guardProof"`
	PlayerProof        string                  `json:"playerProof"`
	Weekly             []P13UFFunctionalPeriod `json:"weekly"`
	Monthly            []P13UFFunctionalPeriod `json:"monthly"`
	Warnings           []string                `json:"warnings,omitempty"`
}

type P13UFFunctionalPeriod struct {
	Period      string `json:"period"`
	Group       string `json:"group,omitempty"`
	PolicyReady bool   `json:"policyReady"`
	OfferReady  bool   `json:"offerReady"`
	Status      string `json:"status"`
}

type p13FunctionalDay struct {
	Battle   []db.P13UnionFightBattleAuditSource
	Guard    []db.P13UnionFightGuardAuditSource
	Factory  []db.P13UnionFightFactorySource
	Cycle    []db.P13UnionFightFactoryCycleSource
	Motion   []db.P13UnionFightCarMotionSource
	Attack   []db.P13UnionFightCarAttackSource
	Tower    []db.P13UnionFightTowerSource
	Life     []db.P13UnionFightItemSource
	GateItem []db.P13UnionFightGateItemSource
	CarItem  []db.P13UnionFightCarItemSource
	Speed    []db.P13UnionFightSpeedItemSource
	Aura     []db.P13UnionFightHealAuraSource
	Drum     []db.P13UnionFightWarDrumSource
	Shop     []db.P13UnionFightShopSource
	Resource []db.P13UnionFightResourceSource
	Revive   []db.P13UnionFightRevivePointSource
}

func p13FunctionalError(message string) error {
	return fmt.Errorf("%w：%s", constUtil.ErrAdminConfiguration, message)
}

// Canonicalize embedded reward/task JSON, while keeping exact protobuf source
// bytes and every typed scalar. Publication timestamps are identity separately.
func p13FunctionalContent(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	delete(m, "published_at")
	delete(m, "publishedAt")
	for _, key := range []string{"rewards", "personal_missions"} {
		if b, ok := m[key]; ok {
			var text string
			if json.Unmarshal(b, &text) == nil && text != "" {
				var nested any
				if json.Unmarshal([]byte(text), &nested) != nil {
					return "", p13FunctionalError(constUtil.P13UFFunctionalConflict)
				}
				normalized, e := json.Marshal(nested)
				if e != nil {
					return "", e
				}
				m[key], e = json.Marshal(string(normalized))
				if e != nil {
					return "", e
				}
			}
		}
	}
	raw, err = json.Marshal(m)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

func p13FunctionalPub(v reflect.Value) int64 { return v.FieldByName("PublishedAt").Int() }

func p13FunctionalWindowEnd() int64 {
	at, _ := time.ParseInLocation("2006-01-02 15:04:05.000", constUtil.P13UFFunctionalWindowEnd, time.FixedZone("Asia/Shanghai", 8*3600))
	return at.UnixMilli()
}

func p13FunctionalProofs() error {
	for _, proof := range []string{constUtil.P6UFGuardMirrorNativeProof, constUtil.P6UFPartyPlayerNativeProof} {
		if len(proof) != 64 {
			return p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		if _, err := hex.DecodeString(proof); err != nil {
			return p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
	}
	return nil
}

func (s *GameService) p13FunctionalDayPlan(day string, pub int64) (*p13FunctionalDay, error) {
	if err := p13FunctionalProofs(); err != nil {
		return nil, err
	}
	if err := s.p13FunctionalOriginal(); err != nil {
		return nil, err
	}
	source := constUtil.P13UFFunctionalSource
	p := &p13FunctionalDay{Battle: []db.P13UnionFightBattleAuditSource{{Day: day, Enabled: true, SeedSalt: 0, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), PublishedAt: pub, Source: source}}}
	for _, family := range []struct {
		prefix string
		n      int
	}{{"tower_guard_", 6}, {"factory_guard_", 2}, {"baseCamp_guard_", 2}} {
		for i := 1; i <= family.n; i++ {
			id := fmt.Sprintf("%s%03d", family.prefix, i)
			monster, err := s.P13UnionFightMirrorGuardTemplate(id)
			if err != nil {
				return nil, err
			}
			raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
			if err != nil {
				return nil, err
			}
			p.Guard = append(p.Guard, db.P13UnionFightGuardAuditSource{GuardRule: constUtil.P13UFGuardMirrorRule, Day: day, GuardID: id, Enabled: true, Monster: raw, PublishedAt: pub, Source: source})
		}
	}
	for i := int64(1); i <= 4; i++ {
		p.Factory = append(p.Factory, db.P13UnionFightFactorySource{Day: day, FactoryID: i, Enabled: true, ChariotHP: 10000, PublishedAt: pub, Source: source})
		p.Cycle = append(p.Cycle, db.P13UnionFightFactoryCycleSource{Day: day, FactoryID: i, Enabled: true, MaxGenerations: 128, MaxAlive: 1, Rule: constUtil.P13UnionFightFactoryBufferRule, BufferCapacity: 1, PublishedAt: pub, Source: source})
		p.Motion = append(p.Motion, db.P13UnionFightCarMotionSource{Day: day, FactoryID: i, Enabled: true, Speed: 2.5, PublishedAt: pub, Source: source})
		p.Attack = append(p.Attack, db.P13UnionFightCarAttackSource{Day: day, FactoryID: i, Enabled: true, HPPercent: 4, Radius: 3.5, PublishedAt: pub, Source: source})
	}
	for i := int64(1); i <= 6; i++ {
		p.Tower = append(p.Tower, db.P13UnionFightTowerSource{Day: day, TowerID: i, Enabled: true, Version: source, HPPercent: 5, PlayerAttack: 65, PlayerDamageRule: constUtil.P13UnionFightTowerPlayerDamageRule, PublishedAt: pub, Source: source})
	}
	for _, item := range []struct {
		id string
		hp int64
	}{{"011", 50}, {"012", 100}, {"016", 20}} {
		p.Life = append(p.Life, db.P13UnionFightItemSource{Day: day, ItemID: "ufitem_" + item.id, Enabled: true, HPPercent: item.hp, PublishedAt: pub, Source: source})
	}
	p.GateItem = []db.P13UnionFightGateItemSource{
		{Day: day, ItemID: "ufitem_019", Enabled: true, HPPercent: 20, BudgetRule: constUtil.P13UFItemBudgetRule, TeamUseLimit: 2, TargetCooldownMillis: 10000, EffectFamily: constUtil.P13UFItemBudgetRepair, PublishedAt: pub, Source: source},
		{Day: day, ItemID: "ufitem_020", Enabled: true, HPPercent: 10, BudgetRule: constUtil.P13UFItemBudgetRule, TeamUseLimit: 4, TargetCooldownMillis: 10000, EffectFamily: constUtil.P13UFItemBudgetHammer, PublishedAt: pub, Source: source},
	}
	p.CarItem = []db.P13UnionFightCarItemSource{
		{Day: day, ItemID: "ufitem_017", Enabled: true, HPPercent: 20, BudgetRule: constUtil.P13UFItemBudgetRule, TeamUseLimit: 4, TargetCooldownMillis: 10000, EffectFamily: constUtil.P13UFItemBudgetBomb, PublishedAt: pub, Source: source},
		{Day: day, ItemID: "ufitem_018", Enabled: true, HPPercent: 40, BudgetRule: constUtil.P13UFItemBudgetRule, TeamUseLimit: 2, TargetCooldownMillis: 10000, EffectFamily: constUtil.P13UFItemBudgetBomb, PublishedAt: pub, Source: source},
	}
	p.Speed = []db.P13UnionFightSpeedItemSource{{Day: day, ItemID: "ufitem_015", Enabled: true, Multiplier: 1.5, AuthorityMask: 6, PublishedAt: pub, Source: source}}
	p.Aura = []db.P13UnionFightHealAuraSource{{Day: day, ItemID: "ufitem_013", Enabled: true, EffectRule: constUtil.P13UFHealAuraPendingRule, HPPercent: 2, Radius: 3, TickMillis: 1000, AuthorityMask: 6, PublishedAt: pub, Source: source}}
	p.Drum = []db.P13UnionFightWarDrumSource{{Day: day, ItemID: "ufitem_014", Enabled: true, AttackPercent: 10, Radius: 3, DurationMillis: 5000, AuthorityMask: 6, ModelName: constUtil.P13UnionFightWarDrumLocalModel, ModelScale: 1, PublishedAt: pub, Source: source}}
	for i := 16; i <= 20; i++ {
		price := int64(20)
		if i == 18 || i == 19 {
			price = 40
		}
		p.Shop = append(p.Shop, db.P13UnionFightShopSource{Day: day, ShopItemID: fmt.Sprintf("local-own-token-%03d", i), Enabled: true, ItemID: fmt.Sprintf("ufitem_%03d", i), CostResource: "", Price: price, Quantity: 1, BuyCount: 5, BuyCondition: 0, Priority: int64(i - 16), RefreshMillis: 86400000, PublishedAt: pub, Source: source})
	}
	for side := 0; side < 2; side++ {
		for index, z := range []float32{-12, -4, 4, 12} {
			x := float32(0)
			if side == 1 {
				x = 75
				z = -z
			}
			p.Resource = append(p.Resource, db.P13UnionFightResourceSource{Day: day, SourceKey: fmt.Sprintf("local-mirror-%d-%d", side, index+1), Enabled: true, ResourceID: strconv.Itoa(index + 1), X: x, Y: 0, Z: z, RewardRes: []int64{2, 5, 20, 20}[index], PublishedAt: pub, Source: source})
		}
	}
	p.Revive = []db.P13UnionFightRevivePointSource{{Day: day, PointID: 5, Enabled: true, MaxGapMillis: 5000, MaxStepMillis: 1000, PublishedAt: pub, Source: source}}
	return p, nil
}

// Check all fixed original selectors, source identities, timings and geometry
// before writing any policy. No temporary Match/Offer/Actor is manufactured.
func (s *GameService) p13FunctionalOriginal() error {
	reject := func() error { return p13FunctionalError(constUtil.P13UFFunctionalMissing) }
	expect := func(table, key, id string, values map[string]string) bool {
		r := s.table(table, key, id)
		if r == nil {
			return false
		}
		for k, v := range values {
			if r[k] != v {
				return false
			}
		}
		return true
	}
	for _, item := range []struct{ id, kind, selector, effect, condition string }{
		{"011", "1", "0", "buff,uf_buff_001", "0"}, {"012", "1", "0", "buff,uf_buff_007", "0"},
		{"013", "1", "0", "buff,uf_buff_009", "2"}, {"014", "1", "0", "totem,1", "2"}, {"015", "1", "4,0,3", "buff,uf_buff_008", "1"},
		{"016", "0", "0", "buff,uf_buff_015", "0"}, {"017", "0", "1,1,5", "buff,uf_buff_013", "0"}, {"018", "0", "1,1,5", "buff,uf_buff_014", "0"},
		{"019", "0", "2,0,5", "buff,uf_buff_005", "0"}, {"020", "0", "2,1,5", "buff,uf_buff_004", "0"},
	} {
		if !expect("UnionFightItem", "UFItemId", "ufitem_"+item.id, map[string]string{"ItemType": item.kind, "ItemSelector": item.selector, "ItemEffect": item.effect, "ItemUseCondition": item.condition, "ItemCatchMaxNum": "5"}) {
			return reject()
		}
		cfg := s.table("UnionFightItem", "UFItemId", "ufitem_"+item.id)
		if item.kind == "1" && s.table("Item", "itemId", cfg["MainItemId"]) == nil {
			return reject()
		}
	}
	if !expect("UnionFightBuff", "UFBuffId", "uf_buff_008", map[string]string{"BuffClass": "0", "lastTime": "5.0"}) || !expect("UnionFightBuff", "UFBuffId", "uf_buff_009", map[string]string{"BuffClass": "2", "lastTime": "5.0", "buffLastEffect": "recoverMatrix"}) {
		return reject()
	}
	for key, value := range map[string]float64{"UnionFightDuration": 5, "UnionFightArriveGuard": .8, "UnionFightArriveOther": .8, "UnionFightKillPlayerRewardScore": 5, "UnionFightKillPlayerRewardOffer": 5, "UnionFightNpcMinDistance": 3, "UnionFightPlayerUseNormalItemCD": 5, "UnionFightPlayerUseDiamondItemCD": 5, "UnionFightRevivePointOccupySpeed": .2, "UnionFightResNum": 50, "UnionFightArriveResourcePointDistance": 1.3, "UnionFightResMinDistance": 3, "UnionFightResConvertOfferFactor": 3} {
		n, e := strconv.ParseFloat(s.table("GameConfigure", "StaticName", key)["NameValue"], 64)
		if e != nil || n != value {
			return reject()
		}
	}
	if s.table("GameConfigure", "StaticName", "UnionFightReviveTime")["NameValue"] != constUtil.P13UnionFightTowerReviveSeconds {
		return reject()
	}
	for i := 1; i <= 4; i++ {
		if !expect("UnionFightResource", "UFRId", strconv.Itoa(i), map[string]string{"GatherCostTime": "3.0", "GatherTime": "5", "RewardGroupId": strconv.FormatInt([]int64{2, 5, 20, 20}[i-1], 10)}) {
			return reject()
		}
	}
	for _, family := range []struct {
		name, key string
		count     int
	}{{"UnionFightBaseCamp", "UFBId", 2}, {"UnionFightGate", "UFGId", 8}, {"UnionFightTower", "UFTId", 6}, {"UnionFightFactory", "UFFId", 4}, {"UnionFightGuard", "UFGuardId", 10}, {"UnionFightRevivePoint", "UFRPId", 7}} {
		if len(s.Tables[family.name]) != family.count {
			return reject()
		}
		if family.name != "UnionFightGuard" {
			for i := 1; i <= family.count; i++ {
				if s.table(family.name, family.key, strconv.Itoa(i)) == nil {
					return reject()
				}
			}
		}
	}
	bound := map[string]bool{}
	for _, table := range []string{"UnionFightBaseCamp", "UnionFightTower", "UnionFightFactory"} {
		for _, r := range s.Tables[table] {
			id := r["UFGuardId"]
			if id == "" {
				continue
			}
			g := s.table("UnionFightGuard", "UFGuardId", id)
			if g == nil || bound[id] || (r["Camp"] != "0" && r["Camp"] != "1") {
				return reject()
			}
			bound[id] = true
		}
	}
	if len(bound) != 10 {
		return reject()
	}
	for i := 1; i <= 6; i++ {
		if !expect("UnionFightTower", "UFTId", strconv.Itoa(i), map[string]string{"Range": "1.4", "AttackCountDown": "5", "AttackDuration": "1", "AttackRelease": "0.6", "RecoveryTime": "120"}) {
			return reject()
		}
	}
	for _, spec := range []struct{ id, camp, occupy string }{{"1", "0", "0"}, {"3", "1", "0"}, {"5", "2", "1"}} {
		if !expect("UnionFightRevivePoint", "UFRPId", spec.id, map[string]string{"InitCamp": spec.camp, "IsCanOccupy": spec.occupy, "OccupyRange": "3.0"}) {
			return reject()
		}
	}
	minimum, maximum := [2]float64{math.MaxFloat64, math.MaxFloat64}, [2]float64{-math.MaxFloat64, -math.MaxFloat64}
	for _, table := range []string{"UnionFightBaseCamp", "UnionFightGate", "UnionFightTower", "UnionFightFactory", "UnionFightRevivePoint", "UnionFightNpc", "UnionFightGuard"} {
		if len(s.Tables[table]) == 0 {
			return reject()
		}
		for _, r := range s.Tables[table] {
			point, e := fieldPoint(r["Position"])
			if e != nil {
				return e
			}
			for i, v := range []float64{float64(point.GetX()), float64(point.GetZ())} {
				minimum[i] = min(minimum[i], v)
				maximum[i] = max(maximum[i], v)
			}
		}
	}
	for _, x := range []float64{0, 75} {
		for _, z := range []float64{-12, -4, 4, 12} {
			if x < minimum[0]-3 || x > maximum[0]+3 || z < minimum[1]-3 || z > maximum[1]+3 {
				return reject()
			}
		}
	}
	for i := 1; i <= 4; i++ {
		cfg := s.table("UnionFightFactory", "UFFId", strconv.Itoa(i))
		if cfg == nil || cfg["CraftTime"] != "120" || cfg["RebuildTime"] != "300" {
			return reject()
		}
		var points []*pb.PointNew
		for _, text := range append([]string{cfg["CariotBornPos"]}, strings.Split(cfg["CariotMoveNavInfo"], ";")...) {
			p, e := fieldPoint(text)
			if e != nil {
				return e
			}
			points = append(points, p)
		}
		gate := s.table("UnionFightGate", "UFGId", cfg["CariotTargetGateId"])
		car := s.table("UnionFightCariot", "UFCId", cfg["CariotId"])
		if gate == nil || gate["Camp"] == cfg["Camp"] || car == nil || car["AttackCountDown"] != "3.5" || car["AttackRelease"] != "0.6" || car["AttackDuration"] != "1.0" {
			return reject()
		}
		point, e := fieldPoint(gate["Position"])
		if e != nil {
			return e
		}
		if _, e = p13UFCarAttackIntersection(points, point, 3.5); e != nil {
			return e
		}
	}
	return nil
}

type p13FunctionalGlobalChange struct {
	value   any
	missing bool
}

func p13FunctionalSame(a, b any) bool {
	x, e := p13FunctionalContent(a)
	y, e2 := p13FunctionalContent(b)
	return e == nil && e2 == nil && x == y
}

func p13FunctionalDefault(v any) bool {
	switch p := v.(type) {
	case *db.P13UnionFightFieldPolicy:
		expected := db.P13UnionFightFieldPolicy{ID: 1, Source: constUtil.P13UnionFightFieldSource}
		if p.PreparationMillis == 300000 {
			expected.PreparationMillis = 300000
		}
		return reflect.DeepEqual(*p, expected)
	case *db.P13UnionFightChatPolicy:
		return reflect.DeepEqual(*p, db.P13UnionFightChatPolicy{ID: 1, TalkCooldownMillis: constUtil.P13UnionFightTalkCooldownMillis, Source: constUtil.P13UnionFightChatSource})
	case *db.P13UnionFightGuardBattlePolicy:
		return reflect.DeepEqual(*p, db.P13UnionFightGuardBattlePolicy{ID: 1, Version: "guard-pending-v1-default-off"})
	case *db.P13UnionFightPlayerBattlePolicy:
		return reflect.DeepEqual(*p, db.P13UnionFightPlayerBattlePolicy{ID: constUtil.P13UFPlayerPartyPolicyID, Version: constUtil.P13UFPlayerPartyPolicyVersion})
	case *db.P13UnionFightItemPolicy:
		return reflect.DeepEqual(*p, db.P13UnionFightItemPolicy{ID: 1, Source: constUtil.P13UnionFightItemSource})
	case *db.P13UnionFightSettlementPolicy:
		return reflect.DeepEqual(*p, db.P13UnionFightSettlementPolicy{Group: p.Group, Source: constUtil.P13UnionFightSettlementSource})
	}
	return false
}

func p13FunctionalSource(v any) string {
	return reflect.Indirect(reflect.ValueOf(v)).FieldByName("Source").String()
}

func p13FunctionalPending(tx *gorm.DB, now time.Time) error {
	var match db.P13UnionFightMatch
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("end_at>?", now.UnixMilli()).Limit(1).Take(&match).Error
	if err == nil {
		return p13FunctionalError(constUtil.P13UFFunctionalPending)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	pendingCalendar := tx.Model(&db.P13UnionFightSchedule{}).Select("1").Where("day>=? AND second_start_at+duration_millis>?", now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02"), now.UnixMilli()).Where("(registration_day<>'' AND registration_day=p13_union_fight_entries.day) OR (registration_day='' AND first_start_at>=p13_union_fight_entries.deadline AND first_start_at<p13_union_fight_entries.deadline+86400000)")
	var entry db.P13UnionFightEntry
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("guild_id,day").Where("deadline>? OR (match_processed=? AND EXISTS (?))", now.UnixMilli(), false, pendingCalendar).Limit(1).Take(&entry).Error
	if err == nil {
		return p13FunctionalError(constUtil.P13UFFunctionalPending)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *GameService) p13FunctionalSettlement(group string, pub int64) db.P13UnionFightSettlementPolicy {
	i := int(group[0] - 'A')
	return db.P13UnionFightSettlementPolicy{Group: group, Enabled: true, Version: constUtil.P13UFFunctionalSource, Source: constUtil.P13UFFunctionalSource, PublishedAt: pub,
		AbortRule: constUtil.P13UFTechnicalAbortRule, FinalizationGraceMillis: 5000, TerminalRule: constUtil.P13UFMixedTerminalRule, DrawPrestige: 5,
		ParticipationRewards: "contribution,20;item_089,1", VictoryRewards: "contribution,100;item_089,2", TransportMVPRewards: "contribution,50;item_089,3",
		PersonalMissions: `[{"kind":1,"target":50,"score":5},{"kind":2,"target":50,"score":5}]`, TaskRewards: "contribution,30;item_089,2", FactoryMissionPrestige: 5,
		WinPrestige: []int32{50, 40, 30, 20}[i], LosePrestige: []int32{25, 20, 15, 10}[i]}
}

func (s *GameService) p13FunctionalGlobalPlan(tx *gorm.DB, schedule *db.P13UnionFightSchedule, businessNow time.Time, pub int64, summary *P13UFFunctionalRulesSummary) ([]p13FunctionalGlobalChange, error) {
	var guild db.P13GuildPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, 1).Error; err != nil {
		return nil, err
	}
	var enrollment db.P13UnionFightPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&enrollment, 1).Error; err != nil {
		return nil, err
	}
	if enrollment.Source != constUtil.P13UnionFightSource && enrollment.Source != constUtil.P13UFFunctionalSource {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
	}
	if !guild.ManagementEnabled || !guild.EconomyEnabled || !guild.ChatEnabled {
		summary.Warnings = append(summary.Warnings, "公会管理、经济或聊天门禁仍关闭；保留当前选择")
	}
	if !enrollment.EnrollmentEnabled || !enrollment.MatchEnabled || !enrollment.CustomCalendarEnabled {
		summary.Warnings = append(summary.Warnings, "报名、匹配或自定义日历开关仍关闭；保留当前选择")
	}
	end := p13FunctionalWindowEnd()
	source := constUtil.P13UFFunctionalSource
	targets := []any{
		&db.P13UnionFightFieldPolicy{ID: 1, Enabled: true, GuardRule: constUtil.P13UFGuardMirrorRule, GuardHPSource: constUtil.P13UnionFightGuardHPSourceMonster, MapProjection: constUtil.P13UnionFightMapProjectionReFlat, PreparationMillis: 300000, LeftRevivePointID: 1, RightRevivePointID: 3, InitialGuardHP: 0, InitialGateHP: 10000, MoveTolerance: 3, Source: source},
		&db.P13UnionFightChatPolicy{ID: 1, TextEnabled: true, TalkEnabled: true, TalkCooldownMillis: constUtil.P13UnionFightTalkCooldownMillis, Source: source},
		&db.P13UnionFightGuardBattlePolicy{ID: 1, Enabled: true, Version: source, Source: source, PublishedAt: pub, StartsAt: pub, EndsAt: end},
		&db.P13UnionFightPlayerBattlePolicy{ID: constUtil.P13UFPlayerPartyPolicyID, Enabled: true, Version: constUtil.P13UFPlayerPartyPolicyVersion, Source: source, PublishedAt: pub, StartsAt: pub, EndsAt: end, ReviveRule: constUtil.P13UFPlayerBattleReviveRule},
		&db.P13UnionFightItemPolicy{ID: 1, Enabled: true, Source: source},
	}
	var settlements []db.P13UnionFightSettlementPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("`group` IN ?", []string{"A", "B", "C", "D"}).Order("`group`").Find(&settlements).Error; err != nil {
		return nil, err
	}
	settlementByGroup := map[string]db.P13UnionFightSettlementPolicy{}
	for _, p := range settlements {
		settlementByGroup[p.Group] = p
	}
	for _, g := range []string{"A", "B", "C", "D"} {
		p := s.p13FunctionalSettlement(g, pub)
		targets = append(targets, &p)
	}
	changes := []p13FunctionalGlobalChange{}
	kept := []any{enrollment}
	for _, target := range targets {
		old := reflect.New(reflect.TypeOf(target).Elem()).Interface()
		missing := false
		if p, ok := target.(*db.P13UnionFightSettlementPolicy); ok {
			existing, found := settlementByGroup[p.Group]
			if found {
				*old.(*db.P13UnionFightSettlementPolicy) = existing
			} else {
				missing = true
			}
		} else {
			id := reflect.Indirect(reflect.ValueOf(target)).FieldByName("ID").Int()
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(old, "id=?", id).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				missing = true
			} else if err != nil {
				return nil, err
			}
		}
		same := !missing && p13FunctionalSame(old, target)
		if !missing {
			switch previous := old.(type) {
			case *db.P13UnionFightGuardBattlePolicy:
				copy := *previous
				copy.StartsAt, copy.EndsAt = pub, end
				same = p13FunctionalSame(&copy, target) && previous.PublishedAt > 0 && previous.PublishedAt <= pub && previous.StartsAt >= previous.PublishedAt && previous.StartsAt <= schedule.FirstStartAt && previous.EndsAt > schedule.SecondStartAt+schedule.DurationMillis+5000
			case *db.P13UnionFightPlayerBattlePolicy:
				copy := *previous
				copy.StartsAt, copy.EndsAt = pub, end
				same = p13FunctionalSame(&copy, target) && previous.PublishedAt > 0 && previous.PublishedAt <= pub && previous.StartsAt >= previous.PublishedAt && previous.StartsAt <= schedule.FirstStartAt && previous.EndsAt > schedule.SecondStartAt+schedule.DurationMillis
			default:
				v := reflect.Indirect(reflect.ValueOf(old))
				field := v.FieldByName("PublishedAt")
				if field.IsValid() {
					same = same && field.Int() > 0 && field.Int() <= pub && field.Int() < schedule.FirstStartAt
				}
			}
		}
		if same {
			kept = append(kept, old)
			continue
		}
		if !missing && p13FunctionalSource(old) != source && !p13FunctionalDefault(old) {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		changes = append(changes, p13FunctionalGlobalChange{value: target, missing: missing})
		kept = append(kept, target)
	}
	if len(changes) > 0 {
		if err := p13FunctionalPending(tx, businessNow); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	summary.GlobalContentHash = p6Digest(raw)
	return changes, nil
}

type p13FunctionalDayInsert struct {
	slice any
	count int
}

// Each typed family is read once under a current read, including disabled and
// extra rows. No per-key query or per-player profile read is used.
func p13FunctionalDayMergePlan(tx *gorm.DB, day string, pub int64, firstStart int64, plan *p13FunctionalDay, summary *P13UFFunctionalRulesSummary) ([]p13FunctionalDayInsert, error) {
	value := reflect.ValueOf(plan).Elem()
	inserts := []p13FunctionalDayInsert{}
	hashes := []string{}
	total := 0
	for i := 0; i < value.NumField(); i++ {
		wanted := value.Field(i)
		expected := map[string]bool{}
		for n := 0; n < wanted.Len(); n++ {
			h, e := p13FunctionalContent(wanted.Index(n).Interface())
			if e != nil {
				return nil, e
			}
			if expected[h] {
				return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
			}
			expected[h] = true
			hashes = append(hashes, h)
			total++
		}
		loaded := reflect.New(wanted.Type())
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("day=?", day).Limit(constUtil.P13UFFunctionalFamilyLimit + 1).Find(loaded.Interface()).Error; err != nil {
			return nil, err
		}
		existing := loaded.Elem()
		if existing.Len() > constUtil.P13UFFunctionalFamilyLimit {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		for n := 0; n < existing.Len(); n++ {
			row := existing.Index(n)
			h, e := p13FunctionalContent(row.Interface())
			if e != nil {
				return nil, e
			}
			if !expected[h] || p13FunctionalPub(row) <= 0 || p13FunctionalPub(row) > pub || p13FunctionalPub(row) >= firstStart {
				return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
			}
			delete(expected, h)
			summary.DayReused++
		}
		missing := reflect.MakeSlice(wanted.Type(), 0, len(expected))
		for n := 0; n < wanted.Len(); n++ {
			h, _ := p13FunctionalContent(wanted.Index(n).Interface())
			if expected[h] {
				missing = reflect.Append(missing, wanted.Index(n))
			}
		}
		if missing.Len() > 0 {
			inserts = append(inserts, p13FunctionalDayInsert{slice: missing.Interface(), count: missing.Len()})
			summary.DayCreated += missing.Len()
		}
	}
	if total != constUtil.P13UFFunctionalDayRows {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalMissing)
	}
	sort.Strings(hashes)
	summary.DayContentHash = p6Digest([]byte(strings.Join(hashes, "\n")))
	return inserts, nil
}

type p13FunctionalPeriods struct {
	weekly       []db.P13UnionFightWeeklyPolicy
	monthly      []db.P13UnionFightMonthlyPolicy
	first        map[string]db.P13UnionFightMatch
	armMonths    []string
	freezeGroups []string
}

func (s *GameService) p13FunctionalRewardSupported(reward string) error {
	ok, err := s.openServerRewardSupported(reward)
	if err != nil {
		return err
	}
	if !ok {
		return p13FunctionalError("原公会战奖励资源尚未完整恢复：" + reward)
	}
	return nil
}

func (s *GameService) p13FunctionalOriginalRewards() (map[string]string, string, error) {
	weekly := map[string][]p13UFWeeklyReward{}
	seen := map[string]map[int32]bool{}
	for _, r := range s.Tables["UnionFightRankReward"] {
		typ, e := strconv.Atoi(r["type"])
		rank, e2 := strconv.ParseInt(r["rank"], 10, 32)
		if e != nil || e2 != nil || typ < 1 || typ > 4 || rank < 1 || rank > 20 {
			return nil, "", p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		group := string(rune('A' + typ - 1))
		if seen[group] == nil {
			seen[group] = map[int32]bool{}
		}
		if seen[group][int32(rank)] {
			return nil, "", p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		seen[group][int32(rank)] = true
		if err := s.p13FunctionalRewardSupported(r["reward"]); err != nil {
			return nil, "", err
		}
		weekly[group] = append(weekly[group], p13UFWeeklyReward{Rank: int32(rank), Rewards: r["reward"]})
	}
	result := map[string]string{}
	for _, g := range []string{"A", "B", "C", "D"} {
		if len(weekly[g]) != 20 {
			return nil, "", p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		sort.Slice(weekly[g], func(i, j int) bool { return weekly[g][i].Rank < weekly[g][j].Rank })
		raw, e := json.Marshal(weekly[g])
		if e != nil {
			return nil, "", e
		}
		result[g] = string(raw)
	}
	monthly := []p13UFWeeklyReward{}
	seenMonth := map[int32]bool{}
	for _, r := range s.Tables["UnionFightRankSpecialReward"] {
		rank, e := strconv.ParseInt(r["rank"], 10, 32)
		if e != nil || rank < 1 || rank > 20 || seenMonth[int32(rank)] {
			return nil, "", p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		seenMonth[int32(rank)] = true
		if err := s.p13FunctionalRewardSupported(r["reward"]); err != nil {
			return nil, "", err
		}
		monthly = append(monthly, p13UFWeeklyReward{Rank: int32(rank), Rewards: r["reward"]})
	}
	if len(monthly) != 20 {
		return nil, "", p13FunctionalError(constUtil.P13UFFunctionalMissing)
	}
	sort.Slice(monthly, func(i, j int) bool { return monthly[i].Rank < monthly[j].Rank })
	raw, err := json.Marshal(monthly)
	if err != nil {
		return nil, "", err
	}
	// Validate single-round compatibility rewards before any Source/Policy write.
	for _, g := range []string{"A", "B", "C", "D"} {
		p := s.p13FunctionalSettlement(g, 1)
		if !p13UFAbortRuleValid(&p) {
			return nil, "", p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		if _, e := p13UFMissionCatalog(&p); e != nil {
			return nil, "", e
		}
		for _, reward := range []string{p.ParticipationRewards, p.VictoryRewards, p.TransportMVPRewards, p.TaskRewards} {
			if err := s.p13FunctionalRewardSupported(reward); err != nil {
				return nil, "", err
			}
		}
	}
	return result, string(raw), nil
}

func (s *GameService) p13FunctionalMonthlyRules(policy db.P13UnionFightMonthlyPolicy, frozen int64) (*db.P13UnionFightMonthlyOffer, error) {
	start, end, err := p13UFMonth(policy.Month)
	if err != nil {
		return nil, err
	}
	rules := p13UFMonthlyRules{Policy: policy}
	for _, choice := range []struct {
		id       string
		enabled  bool
		rank     int32
		chairman bool
	}{{"title_028", policy.MemberTitle, 20, false}, {"title_029", policy.ChairmanTitle, 5, true}} {
		if !choice.enabled {
			continue
		}
		r := s.table("Title", "titleId", choice.id)
		hours := int64(tableNumber(r, "effectTime"))
		if r == nil || hours <= 0 || hours > 87600 || r["effects"] == "" {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalMissing)
		}
		rules.Titles = append(rules.Titles, p13UFMonthlyTitleRule{ID: choice.id, Effects: r["effects"], Hours: hours, MaxRank: choice.rank, Chairman: choice.chairman})
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	offer := &db.P13UnionFightMonthlyOffer{Month: policy.Month, StartAt: start.UnixMilli(), CutoffAt: end.UnixMilli(), Rules: raw, RulesHash: p6Digest(raw), FrozenAt: frozen}
	_, rewards, err := p13UFMonthlyReadRules(offer)
	if err != nil {
		return nil, err
	}
	for _, r := range rewards {
		if err := s.p13FunctionalRewardSupported(r.Rewards); err != nil {
			return nil, err
		}
	}
	return offer, nil
}

// Only the requested week and at most two months are read. Existing immutable
// offers are verified independently of live policy enable/disable switches.
func (s *GameService) p13FunctionalPeriodPlan(tx *gorm.DB, schedule *db.P13UnionFightSchedule, now time.Time, pub int64, summary *P13UFFunctionalRulesSummary) (*p13FunctionalPeriods, error) {
	weeklyRewards, monthlyRewards, err := s.p13FunctionalOriginalRewards()
	if err != nil {
		return nil, err
	}
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	day, err := time.ParseInLocation("2006-01-02", schedule.Day, zone)
	if err != nil {
		return nil, err
	}
	week := p13UFWeek(day)
	weekStart, err := p13UFWeeklyStart(week)
	if err != nil {
		return nil, err
	}
	p := &p13FunctionalPeriods{first: map[string]db.P13UnionFightMatch{}}
	var policies []db.P13UnionFightWeeklyPolicy
	var offers []db.P13UnionFightWeeklyOffer
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("week_start=?", week).Limit(5).Find(&policies).Error; err != nil {
		return nil, err
	}
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("week_start=?", week).Limit(5).Find(&offers).Error; err != nil {
		return nil, err
	}
	if len(policies) > 4 || len(offers) > 4 {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
	}
	policyByGroup := map[string]db.P13UnionFightWeeklyPolicy{}
	offerByGroup := map[string]db.P13UnionFightWeeklyOffer{}
	for _, q := range policies {
		if !strings.Contains("ABCD", q.Group) || len(q.Group) != 1 || q.PublishedAt <= 0 || q.PublishedAt > pub {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		policyByGroup[q.Group] = q
		checked := q
		checked.Enabled = true
		raw, e := json.Marshal(checked)
		if e != nil {
			return nil, e
		}
		_, rewards, e := p13UFWeeklyPolicyPayload(&db.P13UnionFightWeeklyOffer{WeekStart: week, Group: q.Group, Rules: raw, RulesHash: p6Digest(raw), FrozenAt: q.PublishedAt})
		if e != nil {
			return nil, e
		}
		for _, r := range rewards {
			if e = s.p13FunctionalRewardSupported(r.Rewards); e != nil {
				return nil, e
			}
		}
	}
	for _, q := range offers {
		if !strings.Contains("ABCD", q.Group) || len(q.Group) != 1 || q.FrozenAt > now.UnixMilli() {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		_, rewards, e := p13UFWeeklyRules(&q)
		if e != nil {
			return nil, e
		}
		for _, r := range rewards {
			if e = s.p13FunctionalRewardSupported(r.Rewards); e != nil {
				return nil, e
			}
		}
		offerByGroup[q.Group] = q
	}
	// Window selection returns at most one actual match per group, not every
	// week's roster, profile or large frozen entry table.
	ranked := tx.Model(&db.P13UnionFightMatch{}).Select("id, ROW_NUMBER() OVER (PARTITION BY `group` ORDER BY start_at,id) AS source_rank").Where("day>=? AND day<? AND `group` IN ?", week, weekStart.AddDate(0, 0, 7).Format("2006-01-02"), []string{"A", "B", "C", "D"})
	firstIDs := tx.Table("(?) AS earliest", ranked).Select("id").Where("source_rank=1")
	var matches []db.P13UnionFightMatch
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN (?)", firstIDs).Find(&matches).Error; err != nil {
		return nil, err
	}
	if len(matches) > 4 {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
	}
	for _, m := range matches {
		p.first[m.Group] = m
	}
	if len(offers) > 0 {
		ids := []string{}
		for _, q := range offers {
			ids = append(ids, q.FirstMatchID)
		}
		var anchors []db.P13UnionFightMatch
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Find(&anchors).Error; err != nil {
			return nil, err
		}
		byID := map[string]db.P13UnionFightMatch{}
		for _, m := range anchors {
			byID[m.ID] = m
		}
		for _, q := range offers {
			m, ok := byID[q.FirstMatchID]
			if !ok || m.Group != q.Group || m.Day < week || m.Day >= weekStart.AddDate(0, 0, 7).Format("2006-01-02") || q.FrozenAt >= m.StartAt || m.SnapshotHash != p6Digest(m.Roster) {
				return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
			}
		}
	}
	for _, g := range []string{"A", "B", "C", "D"} {
		state := P13UFFunctionalPeriod{Period: week, Group: g, Status: "policy-ready-await-real-match"}
		q, hasPolicy := policyByGroup[g]
		offer, hasOffer := offerByGroup[g]
		first, hasMatch := p.first[g]
		if hasOffer {
			if offer.FrozenAt >= schedule.FirstStartAt {
				return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
			}
			state.OfferReady = true
			state.Status = "existing-offer-preserved"
		}
		if hasMatch && now.UnixMilli() >= first.StartAt && !hasOffer {
			state.Status = "first-match-started-no-retro-offer"
			state.PolicyReady = hasPolicy && q.Enabled
			summary.Weekly = append(summary.Weekly, state)
			continue
		}
		if !hasPolicy && !hasOffer {
			q = db.P13UnionFightWeeklyPolicy{WeekStart: week, Group: g, Enabled: true, Version: constUtil.P13UFFunctionalSource, Source: constUtil.P13UFFunctionalSource, PublishedAt: pub, Rewards: weeklyRewards[g]}
			p.weekly = append(p.weekly, q)
			hasPolicy = true
		}
		state.PolicyReady = hasPolicy && q.Enabled
		if !hasOffer && hasMatch && state.PolicyReady {
			if pub >= first.StartAt {
				return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
			}
			p.freezeGroups = append(p.freezeGroups, g)
			state.Status = "await-prestart-freeze"
		}
		if !state.PolicyReady && !hasOffer {
			state.Status = "existing-disabled-policy-preserved"
		}
		summary.Weekly = append(summary.Weekly, state)
	}
	monthStart := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, zone)
	months := []string{monthStart.Format("2006-01")}
	if monthStart.AddDate(0, 1, 0).Year() <= 9999 {
		months = append(months, monthStart.AddDate(0, 1, 0).Format("2006-01"))
	}
	var monthlyPolicies []db.P13UnionFightMonthlyPolicy
	var monthlyOffers []db.P13UnionFightMonthlyOffer
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("month IN ?", months).Find(&monthlyPolicies).Error; err != nil {
		return nil, err
	}
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("month IN ?", months).Find(&monthlyOffers).Error; err != nil {
		return nil, err
	}
	mp := map[string]db.P13UnionFightMonthlyPolicy{}
	mo := map[string]db.P13UnionFightMonthlyOffer{}
	for _, q := range monthlyPolicies {
		if q.PublishedAt <= 0 || q.PublishedAt > pub {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		mp[q.Month] = q
		checked := q
		checked.Enabled = true
		start, _, e := p13UFMonth(q.Month)
		if e != nil {
			return nil, e
		}
		if q.PublishedAt >= start.UnixMilli() {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		if _, e = s.p13FunctionalMonthlyRules(checked, q.PublishedAt); e != nil {
			return nil, e
		}
	}
	for _, q := range monthlyOffers {
		if q.FrozenAt > now.UnixMilli() {
			return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
		}
		_, rewards, e := p13UFMonthlyReadRules(&q)
		if e != nil {
			return nil, e
		}
		for _, r := range rewards {
			if e = s.p13FunctionalRewardSupported(r.Rewards); e != nil {
				return nil, e
			}
		}
		mo[q.Month] = q
	}
	for _, month := range months {
		start, _, e := p13UFMonth(month)
		if e != nil {
			return nil, e
		}
		q, hasPolicy := mp[month]
		_, hasOffer := mo[month]
		state := P13UFFunctionalPeriod{Period: month, PolicyReady: hasPolicy && q.Enabled, OfferReady: hasOffer, Status: "existing-offer-preserved"}
		if !now.Before(start) || pub >= start.UnixMilli() {
			if !hasOffer {
				state.Status = "month-started-no-retro-policy-or-offer"
			}
			summary.Monthly = append(summary.Monthly, state)
			continue
		}
		if !hasPolicy && !hasOffer {
			q = db.P13UnionFightMonthlyPolicy{Month: month, Enabled: true, Version: constUtil.P13UFFunctionalSource, Source: constUtil.P13UFFunctionalSource, PublishedAt: pub, Rewards: monthlyRewards, MemberTitle: true, ChairmanTitle: true}
			if _, e = s.p13FunctionalMonthlyRules(q, now.UnixMilli()); e != nil {
				return nil, e
			}
			p.monthly = append(p.monthly, q)
			state.PolicyReady = true
		}
		if !hasOffer && state.PolicyReady {
			p.armMonths = append(p.armMonths, month)
			state.Status = "await-pre-month-arm"
		} else if !hasOffer {
			state.Status = "existing-disabled-policy-preserved"
		}
		summary.Monthly = append(summary.Monthly, state)
	}
	return p, nil
}

// PrepareP13UnionFightFunctionalRules must receive the caller's real transaction
// after admin authorization, GuildPolicy lock, CAS, overlap and unfrozen signup
// checks. It publishes typed sources only; actual future-month Arm and existing
// real-match WeeklyFreeze are the existing period lifecycles, not fake results.
func (s *GameService) PrepareP13UnionFightFunctionalRules(tx *gorm.DB, schedule *db.P13UnionFightSchedule, businessNow time.Time) (*P13UFFunctionalRulesSummary, error) {
	if tx == nil || tx.Statement == nil {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalMissing)
	}
	if _, ok := tx.Statement.ConnPool.(interface {
		Commit() error
		Rollback() error
	}); !ok {
		return nil, p13FunctionalError("完整公会战方案须与排期在同一事务发布")
	}
	if schedule == nil || !schedule.Enabled {
		return nil, p13FunctionalError("仅启用排期时准备公会战完整来源")
	}
	if err := p13UFCustomScheduleValid(schedule, schedule.PreparationMillis); err != nil {
		return nil, err
	}
	if schedule.EnrollmentStartAt <= businessNow.UnixMilli() || schedule.PreparationMillis != 300000 {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
	}
	if err := ServerGameClockGuardTransaction(tx); err != nil {
		return nil, err
	}
	pub := time.Now().UnixMilli()
	if tx.Dialector.Name() == "mysql" {
		if err := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS SIGNED)").Scan(&pub).Error; err != nil {
			return nil, err
		}
	}
	if pub <= 0 || pub >= schedule.EnrollmentStartAt || schedule.SecondStartAt+schedule.DurationMillis+5000 >= p13FunctionalWindowEnd() {
		return nil, p13FunctionalError(constUtil.P13UFFunctionalConflict)
	}
	freezeNow := businessNow
	// A normal unshifted admin request can spend milliseconds in authorization
	// before DB NOW is sampled. Never freeze a new period before its real pub.
	if freezeNow.UnixMilli() < pub {
		freezeNow = time.UnixMilli(pub)
	}
	tables, err := adminGuildRuntimeTablesTx(tx, s.Tables)
	if err != nil {
		return nil, err
	}
	original := s.withRuntimeTables(tables)
	if err = original.p13FunctionalTablesReady(); err != nil {
		return nil, err
	}
	summary := &P13UFFunctionalRulesSummary{Day: schedule.Day, PublishedAt: pub, GuardProof: constUtil.P6UFGuardMirrorNativeProof, PlayerProof: constUtil.P6UFPartyPlayerNativeProof}
	day, err := original.p13FunctionalDayPlan(schedule.Day, pub)
	if err != nil {
		return nil, err
	}
	globals, err := original.p13FunctionalGlobalPlan(tx, schedule, businessNow, pub, summary)
	if err != nil {
		return nil, err
	}
	inserts, err := p13FunctionalDayMergePlan(tx, schedule.Day, pub, schedule.FirstStartAt, day, summary)
	if err != nil {
		return nil, err
	}
	periods, err := original.p13FunctionalPeriodPlan(tx, schedule, freezeNow, pub, summary)
	if err != nil {
		return nil, err
	}
	selected := map[string][]map[string]string{}
	for name, rows := range original.Tables {
		if strings.HasPrefix(name, "UnionFight") || name == "Hero" || name == "Skill" || name == "Buff" || name == "Item" || name == "Title" || name == "GameConfigure" {
			selected[name] = rows
		}
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return nil, err
	}
	summary.OriginalTablesHash = p6Digest(raw)
	// Every source, period reward and reference was validated before first write.
	for _, change := range globals {
		var e error
		if change.missing {
			e = tx.Create(change.value).Error
		} else {
			e = tx.Save(change.value).Error
		}
		if e != nil {
			return nil, e
		}
		summary.GlobalsChanged++
	}
	for _, insert := range inserts {
		if err = tx.Create(insert.slice).Error; err != nil {
			return nil, err
		}
	}
	if len(periods.weekly) > 0 {
		if err = tx.Create(&periods.weekly).Error; err != nil {
			return nil, err
		}
	}
	if len(periods.monthly) > 0 {
		if err = tx.Create(&periods.monthly).Error; err != nil {
			return nil, err
		}
	}
	for _, g := range periods.freezeGroups {
		match := periods.first[g]
		if err = original.p13UFWeeklyFreeze(tx, &match, freezeNow); err != nil {
			return nil, err
		}
		for i := range summary.Weekly {
			if summary.Weekly[i].Group == g {
				summary.Weekly[i].OfferReady = true
				summary.Weekly[i].Status = "prestart-offer-frozen"
			}
		}
	}
	if len(periods.armMonths) > 0 {
		if err = original.p13UFMonthlyArm(tx, freezeNow, periods.armMonths...); err != nil {
			return nil, err
		}
		for i := range summary.Monthly {
			for _, m := range periods.armMonths {
				if summary.Monthly[i].Period == m {
					summary.Monthly[i].OfferReady = true
					summary.Monthly[i].Status = "pre-month-offer-frozen"
				}
			}
		}
	}
	return summary, nil
}

// Production startup loads these tables. A publisher never performs file IO or
// silently installs a different catalogue while holding the publication lock.
func (s *GameService) p13FunctionalTablesReady() error {
	if len(s.Tables["Title"]) == 0 || len(s.Tables["UnionFightRankReward"]) != 80 || len(s.Tables["UnionFightRankSpecialReward"]) != 20 {
		return p13FunctionalError(constUtil.P13UFFunctionalMissing)
	}
	return nil
}
