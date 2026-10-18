package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) LoadP12FieldPVPTables(root string) error { return s.loadP7Table(root, "Killer") }
func (s *GameService) InitializeP12FieldPVP(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := s.p12FieldPVPKillerTable(); err != nil {
		return err
	}
	if err := tx.AutoMigrate(&db.P12FieldPVPPolicy{}, &db.P12FieldPVPState{}, &db.P12FieldPVPReceipt{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P12FieldPVPPolicy{ID: 1, PolicyName: constUtil.P12FieldPVPPolicyName, Source: constUtil.P12FieldPVPSource}).Error
}

// Missing tables are a closed fast path; initialization never opts users in.
func p12FieldPVPPolicy(tx *gorm.DB) (*db.P12FieldPVPPolicy, error) {
	if !tx.Migrator().HasTable(&db.P12FieldPVPPolicy{}) {
		return nil, errors.New(constUtil.P12FieldPVPClosed)
	}
	var p db.P12FieldPVPPolicy
	if err := tx.First(&p, 1).Error; err != nil {
		return nil, err
	}
	if !p.Enabled || p.PolicyName != constUtil.P12FieldPVPPolicyName || p.Source != constUtil.P12FieldPVPSource {
		return nil, errors.New(constUtil.P12FieldPVPClosed)
	}
	return &p, nil
}
func p12FieldPVPState(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) (*db.P12FieldPVPState, error) {
	var row db.P12FieldPVPState
	err := tx.First(&row, "player_id=?", p.GetPlayerId()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = db.P12FieldPVPState{PlayerID: p.GetPlayerId(), Source: constUtil.P12FieldPVPSource}
		if g.Field != nil {
			row.Kills = g.Field.Side.GetKills()
		}
	} else if err != nil {
		return nil, err
	}
	if row.Source != constUtil.P12FieldPVPSource || row.Kills < 0 || row.DailyKills < 0 || row.LootCoin < 0 || row.RedeemCount < 0 {
		return nil, errors.New(constUtil.P12FieldPVPConflict)
	}
	day := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	if row.Day != day {
		row.Day = day
		row.DailyKills = 0
		row.LootCoin = 0
	}
	return &row, nil
}

// P12FieldPVPRequireIdle runs under the caller's profile lock and covers other
// battle modes without reading disabled-policy tables on legacy installations.
func P12FieldPVPRequireIdle(tx *gorm.DB, id int64, now time.Time) error {
	if !tx.Migrator().HasTable(&db.P12FieldPVPState{}) {
		return nil
	}
	var row db.P12FieldPVPState
	if err := tx.First(&row, "player_id=?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if row.JailUntil > now.UnixMilli() {
		return errors.New(constUtil.P12FieldPVPJailed)
	}
	if row.BusyUntil > now.UnixMilli() {
		return errors.New(constUtil.P12FieldPVPBusy)
	}
	return nil
}
func CheckP12FieldPVPAction(tx *gorm.DB, id int64, now time.Time) error {
	return P12FieldPVPRequireIdle(tx, id, now)
}

// SyncP12FieldPVPPenalty mutates the outer transaction's SAME decoded objects.
// Existing penalties survive policy disablement; no current source profile is
// overwritten by a second decode/save inside this helper.
func (s *GameService) SyncP12FieldPVPPenalty(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) error {
	if !tx.Migrator().HasTable(&db.P12FieldPVPState{}) {
		return nil
	}
	var row db.P12FieldPVPState
	if err := tx.First(&row, "player_id=?", p.GetPlayerId()).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if row.Source != constUtil.P12FieldPVPSource {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	if g.Field != nil && g.Field.Side != nil {
		g.Field.Side.Kills = proto.Int32(row.Kills)
	}
	p.RedeemTime = proto.Int64(row.JailUntil)
	if row.JailUntil > now.UnixMilli() && row.BusyUntil <= now.UnixMilli() && (g.Position.GetScene() != "town_main" || !p12FieldPVPPrisonPoint(g.Position.GetCurrent())) {
		if g.Field == nil || g.Field.Side == nil {
			return errors.New(constUtil.P12FieldPVPConflict)
		}
		if err := s.p12FieldPVPPrisonLocation(p, g); err != nil {
			return err
		}
		g.Field.Side.Kills = proto.Int32(row.Kills)
		life, err := p8LifeState(tx, p.GetPlayerId())
		if err != nil {
			return err
		}
		life.DeathAt = 0
		life.SafeRecoveryAt = now.UnixMilli()
		life.InSafe = false
		if err = tx.Save(life).Error; err != nil {
			return err
		}
	}
	if row.JailUntil > 0 && now.UnixMilli() >= row.JailUntil {
		return s.p12FieldPVPRelease(tx, p, g, &row, now)
	}
	return nil
}

// ShouldAllowP12PrisonTransfer accepts only the original scripted prison
// destination after presentation ends. Ordinary action eligibility remains in
// CheckP12FieldPVPAction; callers must never interpret false as permission.
func ShouldAllowP12PrisonTransfer(tx *gorm.DB, id int64, scene string, point *pb.PointNew, now time.Time) (bool, error) {
	if !tx.Migrator().HasTable(&db.P12FieldPVPState{}) {
		return false, nil
	}
	var row db.P12FieldPVPState
	if err := tx.First(&row, "player_id=?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if row.JailUntil <= now.UnixMilli() {
		return false, nil
	}
	if row.BusyUntil > now.UnixMilli() {
		return false, errors.New(constUtil.P12FieldPVPBusy)
	}
	return scene == "town_main" && p12FieldPVPPrisonPoint(point), nil
}
func ShouldAllowP12PrisonPosition(tx *gorm.DB, id int64, position *pb.Position, now time.Time) (bool, error) {
	if position == nil || position.GetPlayerId() != 0 && position.GetPlayerId() != id {
		return false, nil
	}
	return ShouldAllowP12PrisonTransfer(tx, id, position.GetScene(), position.GetCurrent(), now)
}

// P12FieldPVPPrisonSide keeps the original jailFlag visible on Town self/peer
// frames. Generic Town's default kills0 would clear RCAISelf.isPlayerNeedJail.
// The caller has already synchronized the authoritative penalty on these objects.
func P12FieldPVPPrisonSide(p *pb.Player, g *model.GameState, now time.Time) (*pb.PlayerSide, bool) {
	if p == nil || g == nil || g.Position.GetScene() != "town_main" || p.GetRedeemTime() <= now.UnixMilli() || g.Field == nil || g.Field.Side == nil || g.Field.Side.GetKills() < 6 {
		return nil, false
	}
	return proto.Clone(g.Field.Side).(*pb.PlayerSide), true
}
func p12FieldPVPPrisonPoint(p *pb.PointNew) bool {
	return p != nil && combatFinite(float64(p.GetX())) && combatFinite(float64(p.GetY())) && combatFinite(float64(p.GetZ())) && math.Abs(float64(p.GetX())-constUtil.P12FieldPVPPrisonX) < .05 && math.Abs(float64(p.GetY())-constUtil.P12FieldPVPPrisonY) < .05 && math.Abs(float64(p.GetZ())-constUtil.P12FieldPVPPrisonZ) < .05
}
func (s *GameService) p12FieldPVPPrisonLocation(p *pb.Player, g *model.GameState) error {
	ratio := tableNumber(s.table("GameConfigure", "StaticName", "Resurrection2Safe"), "NameValue")
	if ratio <= 0 || ratio > 1 {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	if err := s.p8Heal(p, g, ratio, true); err != nil {
		return err
	}
	g.Position = &pb.Position{PlayerId: p.PlayerId, Scene: proto.String("town_main"), Current: &pb.PointNew{X: proto.Float32(float32(constUtil.P12FieldPVPPrisonX)), Y: proto.Float32(float32(constUtil.P12FieldPVPPrisonY)), Z: proto.Float32(float32(constUtil.P12FieldPVPPrisonZ))}}
	g.PositionLoading = true
	g.Field.Side.State = proto.Int32(0)
	g.Field.Side.Anger = proto.Float64(0)
	return nil
}
func (s *GameService) p12FieldPVPRelease(tx *gorm.DB, p *pb.Player, g *model.GameState, row *db.P12FieldPVPState, now time.Time) error {
	if g.Field == nil || g.Field.Side == nil {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	// Local release keeps the original town prison location. The original
	// RCAIPrison door opens when SetKillNum(0) clears its jail flag.
	if err := s.p12FieldPVPPrisonLocation(p, g); err != nil {
		return err
	}
	g.Field.Side.Kills = proto.Int32(0)
	row.Kills = 0
	row.JailUntil = 0
	p.RedeemTime = proto.Int64(0)
	life, err := p8LifeState(tx, p.GetPlayerId())
	if err != nil {
		return err
	}
	life.DeathAt = 0
	life.SafeRecoveryAt = now.UnixMilli()
	life.InSafe = true
	life.InvincibleUntil = now.UnixMilli() + constUtil.P8SafeReviveProtectionMS
	if err = tx.Save(life).Error; err != nil {
		return err
	}
	return tx.Save(row).Error
}
func (s *GameService) p12FieldPVPKillerTable() error {
	expected := [][4]string{{"1", "0", "0", "0"}, {"2", "1", "0", "0"}, {"3", "6", "1", "0"}, {"4", "11", "1", "15"}, {"5", "31", "1", "30"}}
	if len(s.Tables["Killer"]) != len(expected) {
		return errors.New("原TableKiller五档缺失")
	}
	for i, row := range s.Tables["Killer"] {
		want := expected[i]
		if row["id"] != want[0] || row["killNum"] != want[1] || row["jailFlag"] != want[2] || row["jailTime"] != want[3] {
			return errors.New("原TableKiller五档规则改变")
		}
	}
	return nil
}
func (s *GameService) p12FieldPVPKiller(kills int32) (bool, int64, error) {
	if err := s.p12FieldPVPKillerTable(); err != nil {
		return false, 0, err
	}
	var selected map[string]string
	for _, row := range s.Tables["Killer"] {
		if int32(tableNumber(row, "killNum")) <= kills && (selected == nil || tableNumber(row, "killNum") > tableNumber(selected, "killNum")) {
			selected = row
		}
	}
	if selected == nil {
		return false, 0, errors.New("原TableKiller缺失")
	}
	return selected["jailFlag"] == "1", int64(tableNumber(selected, "jailTime")) * 60000, nil
}
func (s *GameService) p12FieldPVPParty(p *pb.Player, g *model.GameState, first int32) (*pb.BattlePlayer, error) {
	party, err := s.battlePlayer(p, g)
	if err != nil {
		return nil, err
	}
	side := g.Field.Side
	if !p8SideValid(side) || side.GetCurHp() <= 0 || !combatFinite(side.GetAnger()) || side.GetAnger() < 0 || side.GetAnger() > 300 || len(side.GetExtBattleAdds()) != 0 {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	byPose := map[int32]float64{}
	for _, hp := range side.GetBattlePosHps() {
		byPose[hp.GetBattlePos()] = hp.GetHpPercent()
	}
	for _, hero := range party.Formation {
		ratio, ok := byPose[hero.GetPose()]
		if !ok {
			if hero.GetPose() == 3 && side.BattlePos3 != nil {
				ratio = side.GetBattlePos3()
			} else {
				ratio = side.GetCurHp()
			}
		}
		if len(party.Formation) == 1 && (len(byPose) > 1 || !combatFinite(ratio) || math.Abs(ratio-side.GetCurHp()) > 1e-12) {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
		if !combatFinite(ratio) || ratio < 0 || ratio > 1 {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
		hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp() * ratio)
	}
	party.Inst.Anger = proto.Float64(side.GetAnger())
	party.Inst.FirstOrder = proto.Int32(first)
	return party, nil
}
func p12FieldPVPApplyTerminal(party *pb.BattlePlayer, side *pb.PlayerSide, out *model.CombatFinalHPOutcome, camp int32) error {
	if party == nil || side == nil || out == nil || out.Outcome == nil || camp < 0 || camp > 1 || len(party.Formation) < 1 || len(party.Formation) > 4 {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	next := proto.Clone(side).(*pb.PlayerSide)
	byGUID := map[string]model.CombatFinalActorHP{}
	for _, actor := range out.FinalActors {
		if actor.Camp != camp {
			continue
		}
		if _, duplicate := byGUID[actor.GUID]; duplicate {
			return errors.New(constUtil.P12FieldPVPConflict)
		}
		byGUID[actor.GUID] = actor
	}
	if len(byGUID) != len(party.Formation) {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	positions := []*pb.BattlePosHp{}
	var current, maximum float64
	for _, hero := range party.Formation {
		actor, ok := byGUID[hero.GetGuid()]
		if !ok || actor.Pose != hero.GetPose() || !combatFinite(actor.MaxHP) || actor.MaxHP <= 0 || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return errors.New(constUtil.P12FieldPVPConflict)
		}
		ratio := actor.CurrentHP / actor.MaxHP
		positions = append(positions, &pb.BattlePosHp{BattlePos: proto.Int32(hero.GetPose()), HpPercent: proto.Float64(ratio)})
		// Same persistent per-pose percentage and base-HP weighted aggregate as
		// existing p8Heal. Runtime maxHP may include original temporary talents.
		current += hero.GetProp().GetHp() * ratio
		maximum += hero.GetProp().GetHp()
		switch hero.GetPose() {
		case 3:
			next.BattlePos3 = proto.Float64(ratio)
		case 7:
			next.BattlePos7 = proto.Float64(ratio)
		case 9:
			next.BattlePos9 = proto.Float64(ratio)
		case 13:
			next.BattlePos13 = proto.Float64(ratio)
		}
	}
	if !combatFinite(current) || !combatFinite(maximum) || maximum <= 0 {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	terminalParty := out.Outcome.Attacker
	if camp == 1 {
		terminalParty = out.Outcome.Defender
	}
	ap := terminalParty.GetInst().GetAnger()
	if terminalParty.GetPlayerId() != party.GetPlayerId() || !combatFinite(ap) || ap < 0 || ap > 300 {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	if current == 0 && side.GetKilleds() == math.MaxInt32 {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	next.CurHp = proto.Float64(current / maximum)
	next.MaxHp = proto.Float64(1)
	next.BattlePosHps = positions
	next.Anger = proto.Float64(ap)
	next.State = proto.Int32(0)
	if current == 0 {
		next.State = proto.Int32(constUtil.P8FieldDead)
		next.Killeds = proto.Int32(side.GetKilleds() + 1)
	}
	proto.Reset(side)
	proto.Merge(side, next)
	return nil
}

func (s *GameService) p12FieldPVPQualify(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) (int32, error) {
	if g.Position.GetScene() == "field_001" || s.table("Field", "FieldId", g.Position.GetScene())["allSafe"] == "1" {
		return 2007, nil
	}
	if g.PositionLoading || g.Position.GetScene() != "field_002" || g.Field == nil || g.Field.Scene != "field_002" || g.Field.Side == nil || p.GetLevel() < 25 {
		return 2002, nil
	}
	if g.Position.GetCurrent() == nil || !combatFinite(float64(g.Position.GetCurrent().GetX())) || !combatFinite(float64(g.Position.GetCurrent().GetY())) || !combatFinite(float64(g.Position.GetCurrent().GetZ())) {
		return 2001, nil
	}
	safe, err := s.p8SafeZone(g)
	if err != nil {
		return 0, err
	}
	if safe {
		return 2007, nil
	}
	if g.Run != nil && !g.Run.Complete || g.Field.Fight != nil && !g.Field.Fight.Settled || g.Field.Side.GetState() != 0 || g.Field.Side.GetCurHp() <= 0 {
		return 2002, nil
	}
	if err = s.ensureNoActiveDailyCopy(tx, p.GetPlayerId(), now); err != nil {
		return 2002, nil
	}
	inv, err := P8IsInvincible(tx, p.GetPlayerId(), now)
	if err != nil {
		return 0, err
	}
	if inv {
		return 2007, nil
	}
	return 1, nil
}
func (s *GameService) p12FieldPVPFightAt(ctx context.Context, id int64, session []byte, b *pb.CSSidePvpFightResult, stamp int64, now time.Time) ([]proto.Message, error) {
	if b == nil || id <= 0 || b.GetPlayerId() <= 0 || id == b.GetPlayerId() || stamp < 0 || b.GetFightType() < 0 || b.GetFightType() > 1 || b.GetServerId() != 0 && int64(b.GetServerId()) != s.OriginalServerID() {
		return nil, errors.New("野外PK请求身份/类型无效")
	}
	var response *pb.SCSidePvpFightResult
	fresh := false
	var sidePush *pb.SCFieldPlayerState
	// Presence writers use this mutex. Profiles then lock by ascending ID, before
	// any consistent snapshot or domain row is read, including zero-stamp replay.
	p10SceneMutation.Lock()
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, err := p10LockProfiles(tx, id, b.GetPlayerId()); err != nil {
			return err
		}
		var ownRow, peerRow db.Profile
		if err := tx.First(&ownRow, id).Error; err != nil {
			return err
		}
		if err := tx.First(&peerRow, b.GetPlayerId()).Error; err != nil {
			return err
		}
		if len(session) != 16 || subtle.ConstantTimeCompare(ownRow.Session, session) != 1 {
			return errors.New("当前本地会话无效")
		}
		own, g, err := decodeProfileForCalendarUpdate(tx, &ownRow)
		if err != nil {
			return err
		}
		peer, pg, err := decodeProfileForCalendarUpdate(tx, &peerRow)
		if err != nil {
			return err
		}
		requestRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(b)
		if err != nil {
			return err
		}
		requestHash := p6Digest(requestRaw)
		var previous db.P12FieldPVPReceipt
		q := tx.Where("player_id=? AND opponent_id=?", id, b.GetPlayerId())
		if stamp > 0 {
			q = q.Where("stamp=?", stamp)
		} else {
			q = q.Where("busy_until>?", now.UnixMilli())
		}
		if !tx.Migrator().HasTable(&db.P12FieldPVPReceipt{}) {
			return errors.New(constUtil.P12FieldPVPClosed)
		}
		err = q.Order("created_at DESC").First(&previous).Error
		if err == nil {
			if previous.RequestHash != requestHash {
				return errors.New(constUtil.P12FieldPVPConflict)
			}
			response, _, err = s.p12FieldPVPReadReceipt(tx, &previous)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if _, err = p12FieldPVPPolicy(tx); err != nil {
			return err
		}
		cfg, err := p10SceneConfig(tx)
		if err != nil {
			return err
		}
		if cfg == nil {
			return errors.New(constUtil.P10SceneClosed)
		}
		var op, pp db.P10ScenePresence
		if tx.First(&op, "player_id=?", id).Error != nil || tx.First(&pp, "player_id=?", b.GetPlayerId()).Error != nil {
			return errors.New("双方必须真实在线同屏")
		}
		if _, ok := s.p10SceneLive(tx, op); !ok {
			return errors.New("发起方当前会话已失效")
		}
		if _, ok := s.p10SceneLive(tx, pp); !ok {
			return errors.New("目标当前会话已失效")
		}
		if op.Scene != pp.Scene || op.Line != pp.Line || op.Line < 1 || op.Line > cfg.LineCount {
			return errors.New("双方必须同图同线")
		}
		states := make([]*db.P12FieldPVPState, 2)
		for i, pair := range []struct {
			p *pb.Player
			g *model.GameState
		}{{own, g}, {peer, pg}} {
			states[i], err = p12FieldPVPState(tx, pair.p, pair.g, now)
			if err != nil {
				return err
			}
			if states[i].BusyUntil > now.UnixMilli() || states[i].JailUntil > now.UnixMilli() {
				response = &pb.SCSidePvpFightResult{Code: proto.Int32(2002)}
				return nil
			}
			code, e := s.p12FieldPVPQualify(tx, pair.p, pair.g, now)
			if e != nil {
				return e
			}
			if code != 1 {
				response = &pb.SCSidePvpFightResult{Code: proto.Int32(code)}
				return nil
			}
		}
		a, d := g.Position.GetCurrent(), pg.Position.GetCurrent()
		if math.Pow(float64(a.GetX()-d.GetX()), 2)+math.Pow(float64(a.GetZ()-d.GetZ()), 2) > constUtil.P12FieldPVPRadius*constUtil.P12FieldPVPRadius {
			response = &pb.SCSidePvpFightResult{Code: proto.Int32(2001)}
			return nil
		}
		red, _, err := s.p12FieldPVPKiller(states[1].Kills)
		if err != nil {
			return err
		}
		if b.GetFightType() == 1 && !red || b.GetFightType() == 0 && red {
			return errors.New("当前目标红名类别不匹配")
		}
		limit, err := s.fieldConfigInt("MaxKillLimit")
		if err != nil {
			return err
		}
		if limit != 60 {
			return errors.New(constUtil.P12FieldPVPConflict)
		}
		if int64(states[0].DailyKills) >= limit {
			return errors.New("原每日击败上限已到")
		}
		for _, pair := range []struct {
			p *pb.Player
			g *model.GameState
		}{{own, g}, {peer, pg}} {
			if err = s.PrepareP5GuildPrayerProperties(ctx, tx, pair.p, pair.g, now); err != nil {
				return err
			}
		}
		attacker, err := s.p12FieldPVPParty(own, g, 1)
		if err != nil {
			return err
		}
		defender, err := s.p12FieldPVPParty(peer, pg, 0)
		if err != nil {
			return err
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32+1))
		if err != nil {
			return err
		}
		fightID := uuid.NewString()
		policyHash := p6Digest([]byte(constUtil.P12FieldPVPSource))
		input := model.FrozenBattleInput{Attacker: attacker, Defender: defender, BattleType: 5, Seed: seed.Int64(), MaxRound: 40, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{"fieldPVP": {fightID, requestHash, policyHash, strconv.FormatInt(int64(b.GetFightType()), 10)}, "fieldPVPEconomy": {strconv.FormatInt(own.GetCoin(), 10), strconv.FormatInt(peer.GetCoin(), 10), strconv.FormatInt(states[0].LootCoin, 10)}, "fieldPVPKills": {strconv.FormatInt(int64(states[0].Kills), 10), strconv.FormatInt(int64(states[1].Kills), 10), strconv.FormatInt(int64(states[0].DailyKills), 10), states[0].Day}}}
		frozen, err := s.FreezeBattleInput(input)
		if err != nil {
			return err
		}
		if _, err = P12FieldPVPProofForInput(frozen); err != nil {
			return err
		}
		terminal, err := s.ComputeP12FieldPVPProtocolFinalHP(ctx, frozen)
		if err != nil {
			return err
		}
		if err = ValidateP12FieldPVPCertifiedReceipt(frozen, terminal); err != nil {
			return err
		}
		if terminal.Outcome.Winner != 0 && terminal.Outcome.Winner != 1 {
			return errors.New("未知野外PK终局拒绝入账")
		}
		recordID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P12FieldPVPRecordSource, fightID, input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", recordID).Error; err != nil {
			return err
		}
		if err = p12FieldPVPStoreRecord(tx, &record, terminal, now); err != nil {
			return err
		}
		if err = p12FieldPVPApplyTerminal(frozen.Attacker, g.Field.Side, terminal, 0); err != nil {
			return err
		}
		if err = p12FieldPVPApplyTerminal(frozen.Defender, pg.Field.Side, terminal, 1); err != nil {
			return err
		}
		win := peer.GetPlayerId()
		loot := int64(0)
		if terminal.Outcome.Winner == 0 {
			win = id
			states[0].DailyKills++
			if b.GetFightType() == 0 {
				if states[0].Kills == math.MaxInt32 {
					return errors.New(constUtil.P12FieldPVPConflict)
				}
				states[0].Kills++
				loot, err = s.p12FieldPVPLoot(peer.GetCoin(), states[0].LootCoin)
				if err != nil {
					return err
				}
				if own.GetCoin() > math.MaxInt64-loot {
					return errors.New(constUtil.P12FieldPVPConflict)
				}
				own.Coin = proto.Int64(own.GetCoin() + loot)
				peer.Coin = proto.Int64(peer.GetCoin() - loot)
				states[0].LootCoin += loot
			}
		}
		// Red death applies to either participant, using the pre-reduction jail tier.
		for i, pair := range []struct {
			p *pb.Player
			g *model.GameState
		}{{own, g}, {peer, pg}} {
			st := states[i]
			st.LastFightID = fightID
			st.BusyUntil = now.UnixMilli() + constUtil.P12FieldPVPBusyMS
			if pair.g.Field.Side.GetCurHp() == 0 {
				jailed, jailMS, e := s.p12FieldPVPKiller(st.Kills)
				if e != nil {
					return e
				}
				if jailed {
					if i == 1 && b.GetFightType() == 1 {
						// Local judicial send preserves the pre-death jailFlag until release.
						st.JailUntil = st.BusyUntil + max(jailMS, int64(st.Kills)*180000)
						pair.p.RedeemTime = proto.Int64(st.JailUntil)
					} else {
						reduce, e := s.fieldConfigInt("ReduceKills")
						if e != nil {
							return e
						}
						if reduce != 1 {
							return errors.New(constUtil.P12FieldPVPConflict)
						}
						st.Kills = max(0, st.Kills-int32(reduce))
					}
				}

				life, e := p8LifeState(tx, pair.p.GetPlayerId())
				if e != nil {
					return e
				}
				life.DeathAt = now.UnixMilli()
				life.SafeRecoveryAt = now.UnixMilli()
				life.LastFightID = fightID
				if e = tx.Save(life).Error; e != nil {
					return e
				}
			}
			pair.g.Field.Side.Kills = proto.Int32(st.Kills)
			if err = tx.Save(st).Error; err != nil {
				return err
			}
		}
		response = &pb.SCSidePvpFightResult{FightId: proto.String(fightID), AttPlayerId: proto.Int64(id), DefPlayerId: peer.PlayerId, WinPlayerId: proto.Int64(win), Result: &pb.ResultObject{}, Code: proto.Int32(1)}
		var resources []*pb.BaseResource
		if loot > 0 {
			resources = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(loot))}}
			response.Result.Simps = []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(int32(loot))}}
			response.Looted = resources
		}
		replay := &pb.SidePvpFightData{Id: proto.String(fightID), RandSeed: proto.Int64(frozen.Seed), AppVersion: proto.String(frozen.ClientVersion), ResVersion: proto.String(frozen.ClientVersion), Time: proto.Int64(now.UnixMilli()), MaxRoundCount: proto.Int32(40), AttPlayer: frozen.Attacker, DefPlayer: frozen.Defender, WinPlayerId: proto.Int64(win), AttGet: resources, DefLose: resources, AttFirstPlayerId: proto.Int64(id)}
		winnerName := peer.GetName()
		if win == id {
			winnerName = own.GetName()
		}
		mv := &pb.SidePvpMV{FightId: proto.String(fightID), AttPlayerName: own.Name, DefPlayerName: peer.Name, AttHeroId: frozen.Attacker.Formation[0].Id, DefHeroId: frozen.Defender.Formation[0].Id, Time: proto.Int64(now.UnixMilli()), WinPlayerName: proto.String(winnerName), Resource: resources, AppVersion: replay.AppVersion, ResVersion: replay.ResVersion}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if err != nil {
			return err
		}
		term, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		rraw, err := proto.Marshal(replay)
		if err != nil {
			return err
		}
		mraw, err := proto.Marshal(mv)
		if err != nil {
			return err
		}
		receipt := db.P12FieldPVPReceipt{FightID: fightID, PlayerID: id, OpponentID: peer.GetPlayerId(), FightType: b.GetFightType(), RequestHash: requestHash, PolicyHash: policyHash, ScopeProof: terminal.Outcome.CapabilityHash, P6RecordID: recordID, WinnerID: win, LootCoin: loot, Response: body, ResponseHash: p6Digest(body), Terminal: term, TerminalHash: p6Digest(term), Replay: rraw, ReplayHash: p6Digest(rraw), MV: mraw, MVHash: p6Digest(mraw), CreatedAt: now.UnixMilli(), BusyUntil: now.UnixMilli() + constUtil.P12FieldPVPBusyMS}
		if stamp > 0 {
			receipt.Stamp = &stamp
		}
		if err = tx.Create(&receipt).Error; err != nil {
			return err
		}
		if err = saveProfile(tx, &ownRow, own, g); err != nil {
			return err
		}
		if err = saveProfile(tx, &peerRow, peer, pg); err != nil {
			return err
		}
		fresh = true
		return nil
	})
	p10SceneMutation.Unlock()
	if err != nil {
		return nil, err
	}
	// Send the original result to both actual participants after commit. Ordinary
	// current SCPlayer follows old deltas, preventing retry balance duplication.
	if fresh {
		s.publishP10Private(ctx, b.GetPlayerId(), response)
		sides := []*pb.PlayerSide{}
		for _, pid := range []int64{id, b.GetPlayerId()} {
			var row db.Profile
			if db.GetIns().WithContext(ctx).First(&row, pid).Error == nil {
				_, g, e := decodeProfile(&row)
				if e == nil && g.Field != nil && g.Field.Side != nil {
					sides = append(sides, proto.Clone(g.Field.Side).(*pb.PlayerSide))
				}
			}
		}
		sidePush = &pb.SCFieldPlayerState{Player: sides}
		s.publishP10Private(ctx, b.GetPlayerId(), sidePush)
	}

	own, _, err := s.resourceProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	if fresh {
		peer, _, e := s.resourceProfile(ctx, b.GetPlayerId())
		if e == nil {
			s.publishP10Private(ctx, b.GetPlayerId(), &pb.SCPlayer{Player: peer})
		}
	}
	messages := []proto.Message{response}
	if sidePush != nil {
		messages = append(messages, sidePush)
	}
	messages = append(messages, &pb.SCPlayer{Player: own})
	return messages, nil
}
func (s *GameService) p12FieldPVPLoot(coin, already int64) (int64, error) {
	percent, err := strconv.ParseFloat(s.table("GameConfigure", "StaticName", "Pvp_Lose_Coin_Percent")["NameValue"], 64)
	if err != nil || percent != 0.02 {
		return 0, errors.New(constUtil.P12FieldPVPConflict)
	}
	cap, err := s.fieldConfigInt("Pvp_Lose_Coin_Max")
	if err != nil {
		return 0, err
	}
	daily, err := s.fieldConfigInt("Pvp_Rob_Coin_TotalMax")
	if err != nil {
		return 0, err
	}
	if cap != 50000 || daily != 500000 {
		return 0, errors.New(constUtil.P12FieldPVPConflict)
	}
	if coin < 0 || already < 0 {
		return 0, errors.New(constUtil.P12FieldPVPConflict)
	}
	// Exact integer 2 percent with floor avoids float loss for large wallets.
	return min(coin/50, cap, max(0, daily-already)), nil
}
func p12FieldPVPStoreRecord(tx *gorm.DB, record *db.P6BattleRecord, out *model.CombatFinalHPOutcome, now time.Time) error {
	input, err := decodeP6Record(record)
	if err != nil {
		return err
	}
	if err = ValidateP12FieldPVPCertifiedReceipt(input, out); err != nil {
		return err
	}
	if record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P12FieldPVPRecordSource {
		return errors.New(constUtil.P12FieldPVPConflict)
	}
	ops, err := proto.Marshal(out.OriginalOperates)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(out.Outcome)
	if err != nil {
		return err
	}
	result := p6PlaybackResult(out.Outcome.Winner)
	return tx.Model(record).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": ops, "outcome": raw, "outcome_hash": p6Digest(raw), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), ops...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()}).Error
}
func (s *GameService) p12FieldPVPReadReceipt(tx *gorm.DB, row *db.P12FieldPVPReceipt) (*pb.SCSidePvpFightResult, *pb.SidePvpFightData, error) {
	bad := func() (*pb.SCSidePvpFightResult, *pb.SidePvpFightData, error) {
		return nil, nil, errors.New(constUtil.P12FieldPVPConflict)
	}
	if row.PolicyHash != p6Digest([]byte(constUtil.P12FieldPVPSource)) || row.ScopeProof != constUtil.P12FieldPVPNativeProof || p6Digest(row.Response) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash || p6Digest(row.Replay) != row.ReplayHash || p6Digest(row.MV) != row.MVHash || row.BusyUntil != row.CreatedAt+constUtil.P12FieldPVPBusyMS {
		return bad()
	}
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, nil, err
	}
	var out model.CombatFinalHPOutcome
	response, replay := &pb.SCSidePvpFightResult{}, &pb.SidePvpFightData{}
	if json.Unmarshal(row.Terminal, &out) != nil || proto.Unmarshal(row.Response, response) != nil || proto.Unmarshal(row.Replay, replay) != nil {
		return bad()
	}
	if err = ValidateP12FieldPVPCertifiedReceipt(input, &out); err != nil {
		return nil, nil, err
	}
	winner := row.PlayerID
	if out.Outcome.Winner == 1 {
		winner = row.OpponentID
	}
	meta := input.AppliedProperties["fieldPVP"]
	if len(meta) != 4 || meta[0] != row.FightID || meta[1] != row.RequestHash || meta[2] != row.PolicyHash || meta[3] != strconv.FormatInt(int64(row.FightType), 10) || record.PlayerID != row.PlayerID || record.Source != constUtil.P12FieldPVPRecordSource || record.SourceFight != row.FightID || record.Status != constUtil.P6BattleComputed || !record.Authoritative || record.OutcomeHash != p6Digest(record.Outcome) || winner != row.WinnerID || input.Attacker.GetPlayerId() != row.PlayerID || input.Defender.GetPlayerId() != row.OpponentID {
		return bad()
	}
	ops, err := proto.Marshal(out.OriginalOperates)
	if err != nil {
		return nil, nil, err
	}
	outcome, err := json.Marshal(out.Outcome)
	if err != nil {
		return nil, nil, err
	}
	if p6Digest(outcome) != record.OutcomeHash || p6Digest(ops) != p6Digest(record.Operates) || record.ServerResult != p6PlaybackResult(out.Outcome.Winner) || record.ClientResult != record.ServerResult || record.ReplayHash != p6Digest(append(append([]byte(nil), ops...), byte(record.ServerResult))) {
		return bad()
	}
	if response.GetCode() != 1 || response.GetFightId() != row.FightID || response.GetAttPlayerId() != row.PlayerID || response.GetDefPlayerId() != row.OpponentID || response.GetWinPlayerId() != winner || replay.GetId() != row.FightID || replay.GetRandSeed() != input.Seed || replay.GetMaxRoundCount() != 40 || !proto.Equal(replay.AttPlayer, input.Attacker) || !proto.Equal(replay.DefPlayer, input.Defender) || replay.GetWinPlayerId() != winner {
		return bad()
	}
	mv := &pb.SidePvpMV{}
	if proto.Unmarshal(row.MV, mv) != nil || mv.GetFightId() != row.FightID || mv.GetAttPlayerName() != input.Attacker.GetNickName() || mv.GetDefPlayerName() != input.Defender.GetNickName() || mv.GetAttHeroId() != input.Attacker.Formation[0].GetId() || mv.GetDefHeroId() != input.Defender.Formation[0].GetId() || mv.GetTime() != row.CreatedAt || replay.GetTime() != row.CreatedAt || replay.GetAttFirstPlayerId() != row.PlayerID || replay.GetAppVersion() != input.ClientVersion || replay.GetResVersion() != input.ClientVersion {
		return bad()
	}
	winnerName := input.Defender.GetNickName()
	if winner == row.PlayerID {
		winnerName = input.Attacker.GetNickName()
	}
	if mv.GetWinPlayerName() != winnerName || mv.GetAppVersion() != input.ClientVersion || mv.GetResVersion() != input.ClientVersion {
		return bad()
	}
	if row.LootCoin < 0 || row.LootCoin > 50000 || row.FightType == 1 && row.LootCoin != 0 || row.WinnerID != row.PlayerID && row.LootCoin != 0 {
		return bad()
	}
	before := input.AppliedProperties["fieldPVPEconomy"]
	if len(before) != 3 {
		return bad()
	}
	coins := make([]int64, 3)
	for i, value := range before {
		v, e := strconv.ParseInt(value, 10, 64)
		if e != nil || v < 0 {
			return bad()
		}
		coins[i] = v
	}
	expectedLoot := int64(0)
	if winner == row.PlayerID && row.FightType == 0 {
		frozenService := &GameService{Tables: input.Tables}
		expectedLoot, err = frozenService.p12FieldPVPLoot(coins[1], coins[2])
		if err != nil {
			return bad()
		}
	}
	if row.LootCoin != expectedLoot {
		return bad()
	}
	want := &pb.ResultObject{}
	var resources []*pb.BaseResource
	if row.LootCoin > 0 {
		want.Simps = []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(int32(row.LootCoin))}}
		resources = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(row.LootCoin))}}
	}
	if !proto.Equal(response.Result, want) || len(response.Cost) != 0 || len(response.Looted) != len(resources) || len(replay.AttGet) != len(resources) || len(replay.DefLose) != len(resources) || len(mv.Resource) != len(resources) {
		return bad()
	}
	for i := range resources {
		if !proto.Equal(response.Looted[i], resources[i]) || !proto.Equal(replay.AttGet[i], resources[i]) || !proto.Equal(replay.DefLose[i], resources[i]) || !proto.Equal(mv.Resource[i], resources[i]) {
			return bad()
		}
	}
	return response, replay, nil
}
func (s *GameService) p12FieldPVPRedeemAt(ctx context.Context, id int64, session []byte, b *pb.CSPlayerRedeem, stamp int64, now time.Time) ([]proto.Message, error) {
	if b.GetType() != 0 && b.GetType() != 1 || stamp < 0 {
		return nil, errors.New("原赎回类型无效")
	}
	var out pb.SCPlayerRedeem
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		row, p, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		_, g, err := decodeProfileForCalendarUpdate(tx, row)
		if err != nil {
			return err
		}
		if !tx.Migrator().HasTable(&db.P12FieldPVPState{}) {
			return errors.New(constUtil.P12FieldPVPClosed)
		}
		if err = s.SyncP12FieldPVPPenalty(tx, p, g, now); err != nil {
			return err
		}
		if err = saveProfile(tx, row, p, g); err != nil {
			return err
		}
		st, err := p12FieldPVPState(tx, p, g, now)
		if err != nil {
			return err
		}
		if st.JailUntil == 0 {
			out.RedeemTime = proto.Int64(0)
			return nil
		}
		if st.JailUntil <= now.UnixMilli() {
			if err = s.p12FieldPVPRelease(tx, p, g, st, now); err != nil {
				return err
			}
			out.RedeemTime = proto.Int64(0)
			return saveProfile(tx, row, p, g)
		}
		if st.BusyUntil > now.UnixMilli() {
			return errors.New(constUtil.P12FieldPVPBusy)
		}
		cost, err := s.p12FieldPVPRedeemCost(st)
		if err != nil {
			return err
		}
		out.RedeemTime = proto.Int64(st.JailUntil)
		out.Cost = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(cost))}}
		if b.GetType() == 0 {
			return nil
		}
		if p.GetCoin() < cost {
			return errors.New("原监狱赎回金币不足")
		}
		p.Coin = proto.Int64(p.GetCoin() - cost)
		if st.RedeemCount == math.MaxInt64 {
			return errors.New(constUtil.P12FieldPVPConflict)
		}
		st.RedeemCount++
		if err = s.p12FieldPVPRelease(tx, p, g, st, now); err != nil {
			return err
		}
		out.RedeemTime = proto.Int64(0)
		return saveProfile(tx, row, p, g)
	})
	if err != nil {
		return nil, err
	}
	p, _, err := s.resourceProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return []proto.Message{&out, &pb.SCPlayer{Player: p}}, nil
}
func (s *GameService) p12FieldPVPRedeemCost(st *db.P12FieldPVPState) (int64, error) {
	key := "KillPerCost"
	if st.RedeemCount == 0 {
		key = "FirstRedeemCost"
	}
	resource, err := parsePoolCost(s.table("GameConfigure", "StaticName", key)["NameValue"])
	if err != nil {
		return 0, err
	}
	if resource.GetId() != "coin" || key == "FirstRedeemCost" && resource.GetNum() != 10000 || key == "KillPerCost" && resource.GetNum() != 8334 {
		return 0, errors.New(constUtil.P12FieldPVPConflict)
	}
	cost := int64(resource.GetNum())
	if st.RedeemCount > 0 {
		cost *= int64(st.Kills)
	}
	if cost <= 0 || cost > math.MaxInt32 {
		return 0, errors.New(constUtil.P12FieldPVPConflict)
	}
	return cost, nil
}
func (s *GameService) p12FieldPVPMV(ctx context.Context, id int64, session []byte, request proto.Message) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, _, err := p10VoiceAuth(tx, id, session); err != nil {
			return err
		}
		if !tx.Migrator().HasTable(&db.P12FieldPVPReceipt{}) {
			return errors.New(constUtil.P12FieldPVPClosed)
		}
		q := tx.Where("(player_id=? OR opponent_id=?)", id, id)
		switch b := request.(type) {
		case *pb.CSSideGetPvpMV:
			var row db.P12FieldPVPReceipt
			if err := q.Where("fight_id=?", b.GetFightId()).First(&row).Error; err != nil {
				return err
			}
			_, replay, err := s.p12FieldPVPReadReceipt(tx, &row)
			if err != nil {
				return err
			}
			response = &pb.SCSideGetPvpMV{Data: replay}
		case *pb.CSSideGetPvpMVList:
			var rows []db.P12FieldPVPReceipt
			if err := q.Order("created_at DESC,fight_id DESC").Limit(50).Find(&rows).Error; err != nil {
				return err
			}
			list := &pb.SCSideGetPvpMVList{}
			for _, row := range rows {
				if _, _, err := s.p12FieldPVPReadReceipt(tx, &row); err != nil {
					return err
				}
				mv := &pb.SidePvpMV{}
				if err := proto.Unmarshal(row.MV, mv); err != nil {
					return err
				}
				list.Mvs = append(list.Mvs, mv)
			}
			response = list
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
func (s *GameService) DispatchP12FieldPVP(ctx context.Context, id int64, session []byte, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSSidePvpFightResult:
		out, err := s.p12FieldPVPFightAt(ctx, id, session, b, stamp, LocalPlayNow(ctx))
		return out, true, err
	case *pb.CSPlayerRedeem:
		out, err := s.p12FieldPVPRedeemAt(ctx, id, session, b, stamp, LocalPlayNow(ctx))
		return out, true, err
	case *pb.CSSideGetPvpMVList, *pb.CSSideGetPvpMV:
		out, err := s.p12FieldPVPMV(ctx, id, session, request)
		return out, true, err
	}
	return nil, false, nil
}
