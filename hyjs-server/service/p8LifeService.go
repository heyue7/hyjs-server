package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP8Life(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P8LifeSkill{}, &db.P8LifeState{})
}

// P8HasLifeSkill 只接受实际消耗原技能书后保存的资格，沿调用方事务与上下文读取。
func P8HasLifeSkill(tx *gorm.DB, id int64, skill string) (bool, error) {
	if tx == nil || !tx.Migrator().HasTable(&db.P8LifeSkill{}) {
		return false, nil
	}
	var row db.P8LifeSkill
	err := tx.First(&row, "player_id=? AND skill_id=?", id, skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.BookID != "" && row.LearnedAt > 0 && row.Source != "", nil
}

func (s *GameService) p8LifeMutation(ctx context.Context, id int64, operation string, stamp int64, request, response proto.Message, action func(*gorm.DB, *pb.Player, *model.GameState, int64) error) ([]proto.Message, error) {
	if stamp < 0 || stamp == 0 && operation != constUtil.P8HpRecoverOperation && operation != constUtil.P8ResurrectOperation && operation != constUtil.P8SafeReviveOperation {
		return nil, p8GatherError()
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		raw = []byte{}
	}
	var side *pb.PlayerSide
	err = s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		var err error
		if s.P12FieldPVPReady && (operation == constUtil.P8HpRecoverOperation || operation == constUtil.P8ResurrectOperation || operation == constUtil.P8SafeReviveOperation) {
			if err = CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
				return err
			}
		}
		// 原TCP恢复操作由真实冷却/死亡状态串行消费，不建立虚构时间戳回执。
		if stamp > 0 {
			var receipt db.P8GatherReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND timestamp=?", id, operation, stamp).Error
			if err == nil {
				if !bytes.Equal(raw, receipt.Request) {
					return p8GatherError()
				}
				if g.Field != nil && g.Field.Side != nil {
					side = proto.Clone(g.Field.Side).(*pb.PlayerSide)
				}
				if err := proto.Unmarshal(receipt.Response, response); err != nil {
					return err
				}
				return p8LifeReceiptCurrent(id, g, response)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err = s.cookingOpen(p); err != nil {
			return err
		}
		if err = action(tx, p, g, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err = saveProfile(tx, row, p, g); err != nil {
			return err
		}
		data, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		if g.Field != nil && g.Field.Side != nil {
			side = proto.Clone(g.Field.Side).(*pb.PlayerSide)
		}
		if stamp == 0 {
			return nil
		}
		return tx.Create(&db.P8GatherReceipt{PlayerID: id, Operation: operation, Timestamp: stamp, Request: raw, Response: data}).Error
	})
	if err != nil {
		return nil, err
	}
	out := []proto.Message{response}
	if side != nil {
		out = append(out, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}})
	}
	return out, nil
}

// p8LifeReceiptCurrent checks the state that the original revival callback
// writes. Resource-only food/book replies retain their existing replay rules.
func p8LifeReceiptCurrent(id int64, g *model.GameState, response proto.Message) error {
	var side *pb.PlayerSide
	if g != nil && g.Field != nil {
		side = g.Field.Side
	}
	switch reply := response.(type) {
	case *pb.SCResurrenttion:
		state := reply.GetState()
		if side == nil || state == nil || state.GetPlayerId() != id || side.GetPlayerId() != id ||
			state.GetState() != side.GetState() || state.GetHp() != side.GetCurHp() ||
			state.GetAnger() != side.GetAnger() || state.GetKills() != side.GetKills() {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	case *pb.SCTransfer2Safe:
		// The original free callback switches the current actor to Normal;
		// a later dead or gathering actor must not receive that old switch.
		if reply.GetResult() == 1 && (side == nil || side.GetPlayerId() != id || side.GetCurHp() <= 0 || side.GetState() != constUtil.P8FieldIdle) {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	}
	return nil
}

func (s *GameService) p8SkillBook(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, error) {
	out := &pb.SCItemUse{Get: &pb.ResultObject{}}
	return s.p8LifeMutation(ctx, id, constUtil.P8SkillBookOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		cost := b.GetCostItem()
		if cost.GetNum() != 1 || cost.GetGuid() != "" || b.GetGetItemIndex() != 0 || s.table("Item", "itemId", cost.GetId())["category"] != "15" {
			return p8GatherError()
		}
		var skill string
		for _, row := range s.Tables["ItemUse"] {
			if row["itemId"] == cost.GetId() {
				if skill != "" || row["category"] != "15" || row["odds"] != "100" || row["result"] == "" || strings.ContainsAny(row["result"], ",;|") {
					return p8GatherError()
				}
				skill = row["result"]
			}
		}
		used := false
		for _, table := range []string{"MineSkill", "FishSkill"} {
			for _, row := range s.Tables[table] {
				used = used || row["SkillNeeded"] == skill && skill != ""
			}
		}
		if !used {
			return p8GatherError()
		}
		learned, err := P8HasLifeSkill(tx, id, skill)
		if err != nil || learned {
			return errors.New("该生活技能已经学习或资格缺失")
		}
		if err = deductPoolCost(p, g, cost); err != nil {
			return err
		}
		if err = tx.Create(&db.P8LifeSkill{PlayerID: id, SkillID: skill, BookID: cost.GetId(), LearnedAt: now, Source: constUtil.P8LifeSkillSource}).Error; err != nil {
			return err
		}
		out.Cost = proto.Clone(cost).(*pb.BaseResource)
		return s.RecordP1Event(tx, id, fmt.Sprintf("p8:skill-book:%d", stamp), constUtil.P8EventLifeBook, 1, time.UnixMilli(now))
	})
}

func (s *GameService) p8CookingLevels(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCCookingLevel{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var skills []db.P8LifeSkill
		if err := tx.Where("player_id=?", id).Order("skill_id").Find(&skills).Error; err != nil {
			return err
		}
		for _, skill := range skills {
			if skill.Source == "" || skill.LearnedAt < 1 {
				return p8GatherError()
			}
			out.Skills = append(out.Skills, skill.SkillID)
		}
		var gem db.GemCraftState
		if err := tx.First(&gem, "player_id=?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			gem.Level = 1
		} else if err != nil {
			return err
		}
		var cooking db.CookingCraftState
		if err := tx.First(&cooking, "player_id=?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			cooking.Level = 1
		} else if err != nil {
			return err
		}
		if gem.Level < 1 || gem.Level > 10 || gem.Exp < 0 || cooking.Level < 1 || cooking.Level > 6 || cooking.Exp < 0 {
			return p8GatherError()
		}
		out.Level, out.Exp, out.GemLevel, out.GemExp = proto.Int32(cooking.Level), proto.Int32(cooking.Exp), proto.Int32(gem.Level), proto.Int32(gem.Exp)
		return nil
	})
	return []proto.Message{out}, err
}

func p8LifeState(tx *gorm.DB, id int64) (*db.P8LifeState, error) {
	var state db.P8LifeState
	err := tx.First(&state, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &db.P8LifeState{PlayerID: id, Source: constUtil.P8LifeStateSource}, nil
	}
	if err != nil {
		return nil, err
	}
	if state.Source == "" || state.FoodReadyAt < 0 || state.SafeRecoveryAt < 0 || state.DeathAt < 0 || state.InvincibleUntil < 0 {
		return nil, p8GatherError()
	}
	return &state, nil
}

func p8SideValid(side *pb.PlayerSide) bool {
	if side == nil || math.IsNaN(side.GetCurHp()) || math.IsInf(side.GetCurHp(), 0) || side.GetCurHp() < 0 || side.GetCurHp() > 1 {
		return false
	}
	seen := map[int32]bool{}
	for _, hp := range side.GetBattlePosHps() {
		if hp.GetBattlePos() < 1 || hp.GetBattlePos() > 15 || seen[hp.GetBattlePos()] || math.IsNaN(hp.GetHpPercent()) || math.IsInf(hp.GetHpPercent(), 0) || hp.GetHpPercent() < 0 || hp.GetHpPercent() > 1 {
			return false
		}
		seen[hp.GetBattlePos()] = true
	}
	return true
}

func (s *GameService) p8Heal(p *pb.Player, g *model.GameState, gain float64, replace bool) error {
	if !p8SideValid(g.Field.Side) || math.IsNaN(gain) || math.IsInf(gain, 0) || gain <= 0 || gain > 1 {
		return p8GatherError()
	}
	party, err := s.battlePlayer(p, g)
	if err != nil || len(party.GetFormation()) == 0 {
		return p8GatherError()
	}
	previous := map[int32]float64{}
	for _, hp := range g.Field.Side.GetBattlePosHps() {
		previous[hp.GetBattlePos()] = hp.GetHpPercent()
	}
	var total, maximum float64
	var values []*pb.BattlePosHp
	for _, hero := range party.GetFormation() {
		old, exists := previous[hero.GetPose()]
		if !exists {
			old = g.Field.Side.GetCurHp()
		}
		next := min(1.0, old+gain)
		if replace {
			next = gain
		}
		values = append(values, &pb.BattlePosHp{BattlePos: proto.Int32(hero.GetPose()), HpPercent: proto.Float64(next)})
		total += hero.GetProp().GetHp() * next
		maximum += hero.GetProp().GetHp()
		switch hero.GetPose() {
		case 3:
			g.Field.Side.BattlePos3 = proto.Float64(next)
		case 7:
			g.Field.Side.BattlePos7 = proto.Float64(next)
		case 9:
			g.Field.Side.BattlePos9 = proto.Float64(next)
		case 13:
			g.Field.Side.BattlePos13 = proto.Float64(next)
		}
	}
	if maximum <= 0 || math.IsNaN(maximum) || math.IsInf(maximum, 0) {
		return p8GatherError()
	}
	g.Field.Side.BattlePosHps = values
	g.Field.Side.CurHp = proto.Float64(total / maximum)
	g.Field.Side.MaxHp = proto.Float64(1)
	return nil
}

func (s *GameService) p8SafeZone(g *model.GameState) (bool, error) {
	point := g.Position.GetCurrent()
	if point == nil {
		return false, p8GatherError()
	}
	zone := s.table("Field", "FieldId", g.Position.GetScene())["SafeZone"]
	parts := strings.Split(zone, ":")
	if len(parts) != 2 || parts[0] != "F" {
		return false, p8GatherError()
	}
	corners := strings.Split(parts[1], ";")
	if len(corners) != 2 {
		return false, p8GatherError()
	}
	var x, z [2]float64
	for i, text := range corners {
		pair := strings.Split(text, ",")
		if len(pair) != 2 {
			return false, p8GatherError()
		}
		var err error
		x[i], err = strconv.ParseFloat(pair[0], 64)
		if err != nil {
			return false, err
		}
		z[i], err = strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return false, err
		}
	}
	return float64(point.GetX()) >= min(x[0], x[1]) && float64(point.GetX()) <= max(x[0], x[1]) && float64(point.GetZ()) >= min(z[0], z[1]) && float64(point.GetZ()) <= max(z[0], z[1]), nil
}

// RefreshP8Survival 按原安全区矩形恢复血量，记录首次死亡及恢复时钟，退出区域不累积恢复。
func (s *GameService) RefreshP8Survival(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) ([]proto.Message, error) {
	// Passive queries remain successful; only their HP/revive clock side effects
	// pause while PK owns the side. Real DB failures still propagate.
	if paused, err := s.pauseP12FieldPVPSurvival(tx, p.GetPlayerId(), now); err != nil {
		return nil, err
	} else if paused {
		return nil, nil
	}
	if s.p8GatherMap(g.Position.GetScene()) == nil || !s.P8FieldSceneAvailable(g.Position.GetScene()) || g.Field == nil {
		state, err := p8LifeState(tx, p.GetPlayerId())
		if err != nil {
			return nil, err
		}
		state.InSafe, state.SafeRecoveryAt = false, now.UnixMilli()
		return nil, tx.Save(state).Error
	}
	if !p8SideValid(g.Field.Side) {
		return nil, p8GatherError()
	}
	state, err := p8LifeState(tx, p.GetPlayerId())
	if err != nil {
		return nil, err
	}
	ms := now.UnixMilli()
	safe, err := s.p8SafeZone(g)
	if err != nil {
		return nil, err
	}
	newOutcome := g.Field.Fight != nil && g.Field.Fight.Settled && g.Field.Fight.Start.GetFightId() != state.LastFightID
	if newOutcome {
		state.LastFightID = g.Field.Fight.Start.GetFightId()
		state.SafeRecoveryAt = ms
		state.DeathAt = 0
	}
	if g.Field.Side.GetCurHp() == 0 {
		if state.DeathAt == 0 {
			state.DeathAt = ms
		}
		state.SafeRecoveryAt = ms
	} else if g.Field.Side.GetCurHp() >= 1 || g.Field.Fight != nil && !g.Field.Fight.Settled || !safe || safe != state.InSafe {
		state.SafeRecoveryAt = ms
	}
	state.InSafe = safe
	changed := false
	if safe && g.Field.Side.GetCurHp() > 0 && g.Field.Side.GetCurHp() < 1 && (g.Field.Fight == nil || g.Field.Fight.Settled) {
		pair := strings.Split(s.table("GameConfigure", "StaticName", "HpRecoveryPercent")["NameValue"], ",")
		if len(pair) != 2 {
			return nil, p8GatherError()
		}
		seconds, err := strconv.ParseInt(pair[0], 10, 32)
		if err != nil || seconds < 1 || seconds > 3600 {
			return nil, p8GatherError()
		}
		amount, err := strconv.ParseFloat(pair[1], 64)
		if err != nil || amount <= 0 || amount > 1 {
			return nil, p8GatherError()
		}
		if state.SafeRecoveryAt == 0 {
			state.SafeRecoveryAt = ms
		}
		steps := (ms - state.SafeRecoveryAt) / (seconds * 1000)
		if steps > 0 {
			if err = s.p8Heal(p, g, min(1.0, float64(steps)*amount), false); err != nil {
				return nil, err
			}
			state.SafeRecoveryAt += steps * seconds * 1000
			changed = true
		}
	}
	if err = tx.Save(state).Error; err != nil {
		return nil, err
	}
	if changed {
		return []proto.Message{&pb.SCFieldPlayerState{Player: []*pb.PlayerSide{g.Field.Side}}}, nil
	}
	return nil, nil
}

func (s *GameService) p8LifeField(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) (*db.P8LifeState, error) {
	if s.p8GatherMap(g.Position.GetScene()) == nil || !s.P8FieldSceneAvailable(g.Position.GetScene()) || g.Run != nil && !g.Run.Complete {
		return nil, p8GatherError()
	}
	if err := s.refreshField(p.GetPlayerId(), g); err != nil {
		return nil, err
	}
	if _, err := s.RefreshP8Survival(tx, p, g, time.UnixMilli(now)); err != nil {
		return nil, err
	}
	if g.Field.Fight != nil && !g.Field.Fight.Settled {
		return nil, errors.New("战斗中不能使用野外生存功能")
	}
	return p8LifeState(tx, p.GetPlayerId())
}

func (s *GameService) p8HpRecover(ctx context.Context, id int64, b *pb.CSHpRecover, stamp int64) ([]proto.Message, error) {
	out := &pb.SCHpRecover{}
	return s.p8LifeMutation(ctx, id, constUtil.P8HpRecoverOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		state, err := s.p8LifeField(tx, p, g, now)
		if err != nil {
			return err
		}
		food := b.GetCostItem()
		percent, err := s.p8FoodPercent(food.GetId())
		if err != nil || food.GetNum() != 1 || food.GetGuid() != "" || g.Field.Side.GetCurHp() <= 0 || g.Field.Side.GetCurHp() >= 1 || g.Field.Side.GetState() == constUtil.P8FieldFishing || g.Field.Side.GetState() == constUtil.P8FieldMining || state.FoodReadyAt > now {
			return p8GatherError()
		}
		seconds, err := s.fieldConfigInt("FieldHpCD")
		if err != nil || seconds > 3600 {
			return p8GatherError()
		}
		if strings.HasPrefix(food.GetId(), "FishItem_") {
			err = s.cookingInventoryDelta(p, g, map[string]int64{food.GetId(): -1})
		} else {
			err = deductPoolCost(p, g, food)
		}
		if err != nil {
			return err
		}
		if err = s.p8Heal(p, g, percent/100, false); err != nil {
			return err
		}
		state.FoodReadyAt = now + seconds*1000
		g.Field.Side.HpRecoverTime = proto.Int64(now)
		if err = tx.Save(state).Error; err != nil {
			return err
		}
		out.Cost = []*pb.BaseResource{proto.Clone(food).(*pb.BaseResource)}
		source := fmt.Sprintf("p8:food:%d", stamp)
		if stamp == 0 {
			source = fmt.Sprintf("p8:food:ready:%d", state.FoodReadyAt)
		}
		return s.RecordP1Event(tx, id, source, constUtil.P8EventHpRecover, 1, time.UnixMilli(now))
	})
}

func (s *GameService) p8Resurrect(ctx context.Context, id int64, b *pb.CSResurrection, stamp int64) ([]proto.Message, error) {
	out := &pb.SCResurrenttion{}
	return s.p8LifeMutation(ctx, id, constUtil.P8ResurrectOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		state, err := s.p8LifeField(tx, p, g, now)
		if err != nil {
			return err
		}
		if g.Field.Side.GetCurHp() != 0 || g.Field.Side.GetState() != constUtil.P8FieldDead {
			return p8GatherError()
		}
		deathAt := state.DeathAt
		costs, err := p3Resources(s.table("GameConfigure", "StaticName", "FieldBuyLife")["NameValue"])
		if err != nil || len(costs) != 1 || costs[0].GetId() != "diamond" {
			return p8GatherError()
		}
		for _, cost := range costs {
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
		}
		if err = s.P8CancelGather(tx, id, g); err != nil {
			return err
		}
		if err = s.p8Heal(p, g, 1, true); err != nil {
			return err
		}
		g.Field.Side.State, g.Field.Side.Anger = proto.Int32(constUtil.P8FieldIdle), proto.Float64(0)
		seconds, err := s.fieldConfigInt("InvincibleTime")
		if err != nil || seconds > 3600 {
			return p8GatherError()
		}
		state.DeathAt, state.SafeRecoveryAt, state.InvincibleUntil = 0, now, now+seconds*1000
		if err = tx.Save(state).Error; err != nil {
			return err
		}
		out.Cost = costs
		out.State = &pb.PushState{PlayerId: proto.Int64(id), State: proto.Int32(constUtil.P8FieldIdle), Hp: proto.Float64(1), Anger: proto.Float64(0), Kills: proto.Int32(g.Field.Side.GetKills())}
		source := fmt.Sprintf("p8:resurrect:%d", stamp)
		if stamp == 0 {
			source = fmt.Sprintf("p8:resurrect:death:%d", deathAt)
		}
		return s.RecordP1Cost(tx, id, source, costs, time.UnixMilli(now))
	})
}

func (s *GameService) p8SafeRevive(ctx context.Context, id int64, b *pb.CSTransfer2Safe, stamp int64) ([]proto.Message, error) {
	out := &pb.SCTransfer2Safe{}
	return s.p8LifeMutation(ctx, id, constUtil.P8SafeReviveOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		state, err := s.p8LifeField(tx, p, g, now)
		if err != nil {
			return err
		}
		if g.Field.Side.GetCurHp() != 0 || g.Field.Side.GetState() != constUtil.P8FieldDead || state.DeathAt <= 0 || now-state.DeathAt < constUtil.P8FreeReviveWaitMS {
			return p8GatherError()
		}
		percent := tableNumber(s.table("GameConfigure", "StaticName", "Resurrection2Safe"), "NameValue")
		if percent <= 0 || percent > 1 {
			return p8GatherError()
		}
		point, err := fieldPoint(s.table("Field", "FieldId", g.Position.GetScene())["Resurrection"])
		if err != nil {
			return err
		}
		if err = s.P8CancelGather(tx, id, g); err != nil {
			return err
		}
		if err = s.p8Heal(p, g, percent, true); err != nil {
			return err
		}
		g.Position.Current = point
		g.Field.Side.State, g.Field.Side.Anger = proto.Int32(constUtil.P8FieldIdle), proto.Float64(0)
		state.DeathAt, state.SafeRecoveryAt, state.InSafe = 0, now, true
		state.InvincibleUntil = now + constUtil.P8SafeReviveProtectionMS
		if err = tx.Save(state).Error; err != nil {
			return err
		}
		out.Result = proto.Int32(1)
		return nil
	})
}

func (s *GameService) p8OwnHp(ctx context.Context, id int64, b *pb.CSGetHp) ([]proto.Message, error) {
	if b.GetPlayerId() != 0 && b.GetPlayerId() != id {
		return s.p8VisibleHp(ctx, id, b.GetPlayerId())
	}
	out := &pb.SCGetHp{}
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		if _, err := s.p8LifeField(tx, p, g, time.Now().UnixMilli()); err != nil {
			return err
		}
		out.Hp = proto.Float64(g.Field.Side.GetCurHp())
		return saveProfile(tx, row, p, g)
	})
	return []proto.Message{out}, err
}

// p8VisibleHp 复用真实同屏资格，只读取当前存档，不用缓存角色帧替代已保存血量。
func (s *GameService) p8VisibleHp(ctx context.Context, id, targetID int64) ([]proto.Message, error) {
	// 换线、消失与查询在同一场景窗口内完成，避免读取一半更新的分线状态。
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	var hp float64
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		cfg, err := p10SceneConfig(tx)
		if err != nil {
			return err
		}
		if cfg == nil || targetID <= 0 {
			return errors.New(constUtil.P8VisibleHpUnavailable)
		}
		var own, target db.P10ScenePresence
		if err = tx.First(&own, "player_id=?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(constUtil.P8VisibleHpUnavailable)
			}
			return err
		}
		if err = tx.First(&target, "player_id=?", targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(constUtil.P8VisibleHpUnavailable)
			}
			return err
		}
		if !constUtil.P8PackagedFieldScene(own.Scene) || own.Scene != target.Scene || own.Line != target.Line || own.Line < 1 || own.Line > cfg.LineCount {
			return errors.New(constUtil.P8VisibleHpUnavailable)
		}
		for _, presence := range []db.P10ScenePresence{own, target} {
			// 两方都须有当前服务的真实在线会话，旧session和离线残帧无查询资格。
			if _, live := s.p10SceneLive(tx, presence); !live {
				return errors.New(constUtil.P8VisibleHpUnavailable)
			}
			var row db.Profile
			if err = tx.First(&row, presence.PlayerID).Error; err != nil {
				return err
			}
			_, game, decodeErr := decodeProfile(&row)
			if decodeErr != nil {
				return decodeErr
			}
			if game.PositionLoading || p10SceneScope(game) != presence.Scene || game.Field == nil || game.Field.Scene != presence.Scene || game.Field.Side == nil || game.Field.Side.GetPlayerId() != presence.PlayerID {
				return errors.New(constUtil.P8VisibleHpUnavailable)
			}
			if presence.PlayerID == targetID {
				hp = game.Field.Side.GetCurHp()
				if !combatFinite(hp) || hp < 0 || hp > 1 {
					return errors.New(constUtil.P8VisibleHpUnavailable)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCGetHp{Hp: proto.Float64(hp)}}, nil
}

// P8IsInvincible 为后续多人/权威伤害处理方提供实际持久化的原复活保护时刻。
func P8IsInvincible(tx *gorm.DB, id int64, now time.Time) (bool, error) {
	state, err := p8LifeState(tx, id)
	return err == nil && state.InvincibleUntil > now.UnixMilli(), err
}

func (s *GameService) DispatchP8Life(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var out []proto.Message
	var err error
	switch b := request.(type) {
	case *pb.CSItemUse:
		if s.table("Item", "itemId", b.GetCostItem().GetId())["category"] != "15" {
			return nil, false, nil
		}
		out, err = s.p8SkillBook(ctx, id, b, stamp)
	case *pb.CSCookingLevel:
		out, err = s.p8CookingLevels(ctx, id)
	case *pb.CSGemAndCookingList:
		out, err = s.p8GemCookingList(ctx, id)
	case *pb.CSSideSetShortCut:
		out, err = s.p8Shortcut(ctx, id, b, stamp)
	case *pb.CSHpRecover:
		out, err = s.p8HpRecover(ctx, id, b, stamp)
	case *pb.CSResurrection:
		out, err = s.p8Resurrect(ctx, id, b, stamp)
	case *pb.CSTransfer2Safe:
		out, err = s.p8SafeRevive(ctx, id, b, stamp)
	case *pb.CSGetHp:
		out, err = s.p8OwnHp(ctx, id, b)
	default:
		return nil, false, nil
	}
	return out, true, err
}
