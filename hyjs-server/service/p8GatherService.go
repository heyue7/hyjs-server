package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
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

type p8GatherConfig struct {
	ResourceID string       `json:"resourceId"`
	Kind       string       `json:"kind"`
	Level      int32        `json:"level"`
	GatherMS   int64        `json:"gatherMs"`
	CooldownMS int64        `json:"cooldownMs"`
	Times      int32        `json:"times"`
	Fatigue    int32        `json:"fatigue"`
	Reward     string       `json:"reward"`
	Point      *pb.PointNew `json:"point"`
	Radius     float64      `json:"radius"`
	Source     string       `json:"source"`
}

// InitializeP8Gather 仅创建独立采集状态表，原角色/冶炼/料理数据继续各域管理。
func (s *GameService) InitializeP8Gather(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P8ResourceNode{}, &db.P8GatherAttempt{}, &db.P8GatherReceipt{})
}

func (s *GameService) LoadP8Tables(root string) error {
	for _, name := range []string{"FieldResource", "FieldResourcePos"} {
		file, err := os.Open(filepath.Join(root, "Table"+name+".tsv"))
		if err != nil {
			return err
		}
		reader := csv.NewReader(file)
		reader.Comma, reader.FieldsPerRecord = '\t', -1
		rows, err := reader.ReadAll()
		file.Close()
		if err != nil {
			return err
		}
		if len(rows) < 5 {
			return p8GatherError()
		}
		var entries []map[string]string
		for _, values := range rows[5:] {
			if len(values) == 0 || values[0] == "" {
				continue
			}
			entry := map[string]string{}
			for i, v := range values {
				if i < len(rows[1]) {
					entry[rows[1][i]] = v
				}
			}
			entries = append(entries, entry)
		}
		s.Tables[name] = entries
	}
	return nil
}

func p8GatherError() error { return errors.New("采集条件不满足或原配置缺失") }

func p8DecodeConfig(raw string) (*p8GatherConfig, error) {
	var cfg p8GatherConfig
	if json.Unmarshal([]byte(raw), &cfg) != nil || cfg.Source == "" || cfg.ResourceID == "" || (cfg.Kind != "mine" && cfg.Kind != "fish") || cfg.Level < 1 || cfg.Level > 6 || cfg.GatherMS < 1 || cfg.GatherMS > constUtil.P8GatherMaxDurationMS || cfg.CooldownMS < 1 || cfg.CooldownMS > constUtil.P8GatherMaxDurationMS || cfg.Times < 1 || cfg.Times > 100 || cfg.Fatigue < 1 || cfg.Fatigue > 200 || cfg.Point == nil || cfg.Radius <= 0 || cfg.Radius > 100 || math.IsNaN(cfg.Radius) || math.IsInf(cfg.Radius, 0) || cfg.Reward == "" {
		return nil, p8GatherError()
	}
	return &cfg, nil
}

// p8GatherMap keeps each original map's resource catalogue and scene identity.
// Shared transfer/refresh still verifies the actual supported scene before use.
func (s *GameService) p8GatherMap(scene string) map[string]string {
	row := s.table("Field", "FieldId", scene)
	if row == nil || row["FieldMap"] == "" || row["MinePoints"] == "" || row["FishPoints"] == "" {
		return nil
	}
	return row
}

func (s *GameService) p8SpawnNode(id int64, scene, posID string, generation int64) (*db.P8ResourceNode, error) {
	field := s.p8GatherMap(scene)
	if field == nil {
		return nil, p8GatherError()
	}
	belongs := false
	for _, key := range []string{"MinePoints", "FishPoints"} {
		for _, original := range strings.Split(field[key], ";") {
			belongs = belongs || original == posID
		}
	}
	pos := s.table("FieldResourcePos", "posId", posID)
	if !belongs || pos == nil || generation < 1 {
		return nil, p8GatherError()
	}
	group, err := fieldWeighted(pos["resourceIdProbs"])
	if err != nil {
		return nil, err
	}
	row := s.table("FieldResource", "id", group)
	if row == nil {
		return nil, p8GatherError()
	}
	point, err := fieldPoint(pos["pos"])
	if err != nil {
		return nil, err
	}
	radius, err := strconv.ParseFloat(pos["r"], 64)
	if err != nil {
		return nil, err
	}
	read := func(key string) (int64, error) { return strconv.ParseInt(row[key], 10, 32) }
	level, err := read("level")
	if err != nil {
		return nil, err
	}
	duration, err := read("gatherTime")
	if err != nil {
		return nil, err
	}
	cooldown, err := read("cdTime")
	if err != nil {
		return nil, err
	}
	times, err := read("times")
	if err != nil {
		return nil, err
	}
	costs, err := p3Resources(row["cost"])
	if err != nil || len(costs) != 1 || costs[0].GetId() != "tiredness" {
		return nil, p8GatherError()
	}
	kind := "mine"
	if strings.HasPrefix(posID, "fish_pos_") {
		kind = "fish"
	}
	if !strings.HasPrefix(group, "group_"+kind+"_") {
		return nil, p8GatherError()
	}
	resourceID := group
	if row["adminOriginalID"] != "" {
		resourceID = row["adminOriginalID"]
	}
	cfg := p8GatherConfig{ResourceID: resourceID, Kind: kind, Level: int32(level), GatherMS: duration, CooldownMS: cooldown, Times: int32(times), Fatigue: costs[0].GetNum(), Reward: row["rewardProbId"], Point: point, Radius: radius, Source: constUtil.P8GatherPolicySource + field["adminRevision"]}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if _, err = p8DecodeConfig(string(raw)); err != nil {
		return nil, err
	}
	return &db.P8ResourceNode{PlayerID: id, Scene: scene, PosID: posID, ResourceID: resourceID, Remaining: int32(times), Generation: generation, Snapshot: string(raw)}, nil
}

func (s *GameService) p8GatherOpen(id int64, p *pb.Player, g *model.GameState, accepted ...bool) error {
	row := s.p8GatherMap(g.Position.GetScene())
	frozenClaim := len(accepted) > 0 && accepted[0]
	if row == nil || !constUtil.P8PackagedFieldScene(g.Position.GetScene()) || !frozenClaim && (!s.P8FieldSceneAvailable(g.Position.GetScene()) || p.GetLevel() < int32(tableNumber(row, "levelLimit"))) {
		return p8GatherError()
	}
	if err := s.refreshField(id, g); err != nil {
		return err
	}
	if g.Field.Side.GetCurHp() <= 0 || g.Run != nil && !g.Run.Complete || g.Field.Fight != nil && !g.Field.Fight.Settled {
		return errors.New("倒下或战斗中不能采集")
	}
	return nil
}

// p8Nodes 首次从原候选点无放回随机选择地图上限；耗尽后在同点按原权重重生。
func (s *GameService) p8Nodes(tx *gorm.DB, id int64, scene string, now int64) ([]db.P8ResourceNode, bool, error) {
	row := s.p8GatherMap(scene)
	if row == nil {
		return nil, false, p8GatherError()
	}
	var nodes []db.P8ResourceNode
	if err := tx.Where("player_id=? AND scene=?", id, scene).Order("pos_id").Find(&nodes).Error; err != nil {
		return nil, false, err
	}
	changed := false
	// Preserve a live attempt's frozen node. Once it finishes, replace stale map
	// projections in the same player transaction so count/point edits take effect.
	if row["adminRevision"] != "" && len(nodes) > 0 {
		expected := constUtil.P8GatherPolicySource + row["adminRevision"]
		stale := false
		for _, node := range nodes {
			cfg, e := p8DecodeConfig(node.Snapshot)
			if e != nil {
				return nil, false, e
			}
			stale = stale || cfg.Source != expected
		}
		if stale {
			var active int64
			if tx.Migrator().HasTable(&db.P8GatherAttempt{}) {
				if err := tx.Model(&db.P8GatherAttempt{}).Where("player_id=? AND active=?", id, true).Count(&active).Error; err != nil {
					return nil, false, err
				}
			}
			if active == 0 {
				if err := tx.Where("player_id=? AND scene=?", id, scene).Delete(&db.P8ResourceNode{}).Error; err != nil {
					return nil, false, err
				}
				nodes = nil
				changed = true
			}
		}
	}
	if len(nodes) == 0 {
		for _, spec := range []struct{ points, maximum string }{{"MinePoints", "MineMax"}, {"FishPoints", "FishGroupMax"}} {
			points := strings.Split(row[spec.points], ";")
			if row[spec.points] == "" {
				points = nil
			}
			limit, err := strconv.ParseInt(row[spec.maximum], 10, 32)
			if err != nil || limit < 0 || limit > int64(len(points)) {
				return nil, false, p8GatherError()
			}
			seen := map[string]bool{}
			for _, pos := range points {
				if pos == "" || seen[pos] {
					return nil, false, p8GatherError()
				}
				seen[pos] = true
			}
			for n := int64(0); n < limit; n++ {
				picked, err := gemRoll(int64(len(points)))
				if err != nil {
					return nil, false, err
				}
				node, err := s.p8SpawnNode(id, scene, points[picked], 1)
				if err != nil {
					return nil, false, err
				}
				if err = tx.Create(node).Error; err != nil {
					return nil, false, err
				}
				nodes = append(nodes, *node)
				points = append(points[:picked], points[picked+1:]...)
			}
		}
		changed = true
	}
	for i := range nodes {
		node := &nodes[i]
		cfg, err := p8DecodeConfig(node.Snapshot)
		if err != nil || node.Remaining < 0 || node.Remaining > cfg.Times || node.Generation < 1 {
			return nil, false, p8GatherError()
		}
		if node.Remaining == 0 && node.RefreshAt > 0 && node.RefreshAt <= now {
			if node.Generation == math.MaxInt64 {
				return nil, false, p8GatherError()
			}
			replacement, err := s.p8SpawnNode(id, scene, node.PosID, node.Generation+1)
			if err != nil {
				return nil, false, err
			}
			if err = tx.Save(replacement).Error; err != nil {
				return nil, false, err
			}
			nodes[i] = *replacement
			changed = true
		}
	}
	return nodes, changed, nil
}

func p8ResourceInfo(nodes []db.P8ResourceNode, now int64) *pb.SCFieldResourceInfo {
	out := &pb.SCFieldResourceInfo{}
	for _, node := range nodes {
		refresh := int64(0)
		if node.Remaining == 0 {
			refresh = max(int64(1), (node.RefreshAt-now+999)/1000)
		}
		out.Resource = append(out.Resource, &pb.SideResource{Id: proto.String(node.ResourceID), PosId: proto.String(node.PosID), RefreshTime: proto.Int32(int32(min(int64(math.MaxInt32), refresh)))})
	}
	return out
}

func p8Near(point *pb.PointNew, cfg *p8GatherConfig) bool {
	if point == nil {
		return false
	}
	var distance float64
	for _, delta := range []float64{float64(point.GetX() - cfg.Point.GetX()), float64(point.GetY() - cfg.Point.GetY()), float64(point.GetZ() - cfg.Point.GetZ())} {
		if math.IsNaN(delta) || math.IsInf(delta, 0) {
			return false
		}
		distance += delta * delta
	}
	return distance <= cfg.Radius*cfg.Radius
}

// P8CancelGather 与移动/离场/开战共用原玩家事务；取消不扣疲劳也不生成奖励。
func (s *GameService) P8CancelGather(tx *gorm.DB, id int64, g *model.GameState) error {
	if err := tx.Model(&db.P8GatherAttempt{}).Where("player_id=? AND active=?", id, true).Update("active", false).Error; err != nil {
		return err
	}
	if g.Field != nil && (g.Field.Side.GetState() == constUtil.P8FieldFishing || g.Field.Side.GetState() == constUtil.P8FieldMining) {
		g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	}
	return nil
}

// P8ObserveGatherMove 保留原客户端进入采集动画时补发的点内停止位置。
// 冻结资源半径同时用于开始、移动及领取；走出范围后立即取消，不改变奖励与计时。
func (s *GameService) P8ObserveGatherMove(tx *gorm.DB, id int64, g *model.GameState, next *pb.PointNew) error {
	if g.Field == nil || g.Field.Side == nil || (g.Field.Side.GetState() != constUtil.P8FieldFishing && g.Field.Side.GetState() != constUtil.P8FieldMining) {
		return nil
	}
	var attempt db.P8GatherAttempt
	err := tx.First(&attempt, "player_id=? AND active=?", id, true).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
		return nil
	}
	if err != nil {
		return err
	}
	cfg, err := p8DecodeConfig(attempt.Snapshot)
	if err != nil {
		return err
	}
	if attempt.Scene == g.Position.GetScene() && p8Near(next, cfg) {
		return nil
	}
	return s.P8CancelGather(tx, id, g)
}

// RefreshP8Resources 用于场景恢复和心跳，原刷新帧refreshTime>0移除模型、0重新出现。
func (s *GameService) RefreshP8Resources(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time, force bool) ([]proto.Message, error) {
	if s.p8GatherMap(g.Position.GetScene()) == nil || !s.P8FieldSceneAvailable(g.Position.GetScene()) {
		return nil, nil
	}
	if err := s.refreshField(p.GetPlayerId(), g); err != nil {
		return nil, err
	}
	nodes, changed, err := s.p8Nodes(tx, p.GetPlayerId(), g.Position.GetScene(), now.UnixMilli())
	if err != nil {
		return nil, err
	}
	var attempt db.P8GatherAttempt
	err = tx.First(&attempt, "player_id=? AND active=?", p.GetPlayerId(), true).Error
	if err == nil {
		cfg, err := p8DecodeConfig(attempt.Snapshot)
		if err != nil {
			return nil, err
		}
		if attempt.Scene != g.Position.GetScene() || !p8Near(g.Position.GetCurrent(), cfg) || g.Field.Side.GetCurHp() <= 0 || g.Field.Fight != nil && !g.Field.Fight.Settled {
			if err = s.P8CancelGather(tx, p.GetPlayerId(), g); err != nil {
				return nil, err
			}
			changed = true
		} else if cfg.Kind == "fish" {
			g.Field.Side.State = proto.Int32(constUtil.P8FieldFishing)
		} else {
			g.Field.Side.State = proto.Int32(constUtil.P8FieldMining)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if force || changed {
		return []proto.Message{p8ResourceInfo(nodes, now.UnixMilli()), &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{g.Field.Side}}}, nil
	}
	return nil, nil
}

func (s *GameService) p8LifeLevel(tx *gorm.DB, id int64, kind string) (int32, error) {
	if kind == "mine" {
		var state db.GemCraftState
		err := tx.First(&state, "player_id=?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 1, nil
		}
		if err != nil || state.Level < 1 || state.Level > 10 {
			return 0, p8GatherError()
		}
		return state.Level, nil
	}
	var state db.CookingCraftState
	err := tx.First(&state, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if err != nil || state.Level < 1 || state.Level > 6 {
		return 0, p8GatherError()
	}
	return state.Level, nil
}

func (s *GameService) p8RollGather(cfg *p8GatherConfig) (string, error) {
	var gains []string
	for _, spec := range strings.Split(cfg.Reward, "|") {
		pair := strings.Split(spec, ":")
		if len(pair) != 2 {
			return "", p8GatherError()
		}
		num, err := fieldWeighted(pair[1])
		if err != nil {
			return "", err
		}
		n, err := strconv.ParseInt(num, 10, 32)
		if err != nil || n < 0 {
			return "", p8GatherError()
		}
		if n > 0 {
			gains = append(gains, pair[0]+","+num)
		}
	}
	if len(gains) == 0 && cfg.Kind == "fish" {
		return s.table("GameConfigure", "StaticName", "FishNoReward")["NameValue"], nil
	}
	return strings.Join(gains, ";"), nil
}

func (s *GameService) p8GrantGather(p *pb.Player, g *model.GameState, rewards string) (*pb.ResultObject, error) {
	// A valid weighted mine can yield no resources; the accepted attempt still
	// consumes its original node use and fatigue exactly once.
	if rewards == "" {
		return &pb.ResultObject{}, nil
	}
	mine, fish := map[string]int64{}, map[string]int64{}
	var other []string
	for _, spec := range strings.Split(rewards, ";") {
		pair := strings.Split(spec, ",")
		if len(pair) != 2 {
			return nil, p8GatherError()
		}
		n, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || n < 1 {
			return nil, p8GatherError()
		}
		switch {
		case s.table("MineSkill", "MineItemID", pair[0]) != nil:
			mine[pair[0]] += n
		case s.table("FishSkill", "FishItemID", pair[0]) != nil:
			fish[pair[0]] += n
		case s.table("Item", "itemId", pair[0]) != nil:
			other = append(other, spec)
		default:
			return nil, p8GatherError()
		}
	}
	out := &pb.ResultObject{}
	if len(other) > 0 {
		gain, err := s.GrantFixedReward(p, g, strings.Join(other, ";"))
		if err != nil {
			return nil, err
		}
		redMergeGain(out, gain)
	}
	if err := s.gemInventoryDelta(p, g, mine); err != nil {
		return nil, err
	}
	if err := s.cookingInventoryDelta(p, g, fish); err != nil {
		return nil, err
	}
	redMergeGain(out, gemRewards(mine))
	redMergeGain(out, gemRewards(fish))
	return out, nil
}

func (s *GameService) p8GatherMutation(ctx context.Context, id int64, op string, stamp int64, request, response proto.Message, apply func(*gorm.DB, *pb.Player, *model.GameState, int64) error) ([]proto.Message, error) {
	if stamp < 0 || stamp == 0 && op != constUtil.P8GatherStartOperation && op != constUtil.P8GatherClaimOperation {
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
	var info *pb.SCFieldResourceInfo
	err = s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		var err error
		if s.P12FieldPVPReady {
			if err = CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
				return err
			}
		}
		// 原TCP没有请求时间戳，当前attempt/node事务状态负责防止重复领取。
		// 只给实际HTTP正时间戳保存回执，避免制造客户端不存在的请求键。
		if stamp > 0 {
			var receipt db.P8GatherReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND timestamp=?", id, op, stamp).Error
			if err == nil {
				if !bytes.Equal(receipt.Request, raw) {
					return p8GatherError()
				}
				if g.Field != nil {
					side = proto.Clone(g.Field.Side).(*pb.PlayerSide)
				}
				return proto.Unmarshal(receipt.Response, response)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		now := time.Now().UnixMilli()
		// A new deployment cannot revoke a previously accepted gather. Only the
		// new-open gates are bypassed; the original claim still verifies scene,
		// distance, remaining node, fatigue, death and mutually exclusive fights.
		acceptedClaim := false
		if op == constUtil.P8GatherClaimOperation {
			var active db.P8GatherAttempt
			e := tx.First(&active, "player_id=? AND active=?", id, true).Error
			if e == nil {
				acceptedClaim = active.Scene == g.Position.GetScene()
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		if err = s.p8GatherOpen(id, p, g, acceptedClaim); err != nil {
			return err
		}
		if err = apply(tx, p, g, now); err != nil {
			return err
		}
		if err = saveProfile(tx, row, p, g); err != nil {
			return err
		}
		nodes, _, err := s.p8Nodes(tx, id, g.Position.GetScene(), now)
		if err != nil {
			return err
		}
		info = p8ResourceInfo(nodes, now)
		side = proto.Clone(g.Field.Side).(*pb.PlayerSide)
		if stamp == 0 {
			return nil
		}
		data, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Create(&db.P8GatherReceipt{PlayerID: id, Operation: op, Timestamp: stamp, Request: raw, Response: data}).Error
	})
	if err != nil {
		return nil, err
	}
	out := []proto.Message{response}
	if side != nil {
		out = append(out, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}})
	}
	if info != nil {
		out = append(out, info)
	}
	return out, nil
}

func (s *GameService) p8StartGather(ctx context.Context, id int64, b *pb.CSFieldResourceStartGather, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFieldResourceStartGather{}
	return s.p8GatherMutation(ctx, id, constUtil.P8GatherStartOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		nodes, _, err := s.p8Nodes(tx, id, g.Position.GetScene(), now)
		if err != nil {
			return err
		}
		var node *db.P8ResourceNode
		for i := range nodes {
			if nodes[i].PosID == b.GetPosId() {
				node = &nodes[i]
			}
		}
		if node == nil || node.Remaining == 0 {
			out.Result = proto.Int32(constUtil.P8ResourceMissing)
			return nil
		}
		cfg, err := p8DecodeConfig(node.Snapshot)
		if err != nil {
			return err
		}
		level, err := s.p8LifeLevel(tx, id, cfg.Kind)
		if err != nil {
			return err
		}
		if level < cfg.Level {
			kind := "冶炼"
			if cfg.Kind == "fish" {
				kind = "料理"
			}
			return fmt.Errorf("需要%s等级%d，当前等级%d", kind, cfg.Level, level)
		}
		if g.Field.Side.GetTiredness() < cfg.Fatigue {
			return errors.New("疲劳不足，无法开始采集")
		}
		if !p8Near(g.Position.GetCurrent(), cfg) {
			return errors.New("请靠近资源点后再开始采集")
		}
		var old db.P8GatherAttempt
		err = tx.First(&old, "player_id=?", id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.Active {
			if old.Scene != node.Scene || old.PosID != node.PosID || old.Generation != node.Generation {
				return errors.New("当前已有采集，需要先取消")
			}
		} else {
			token, err := fieldToken()
			if err != nil {
				return err
			}
			rewards, err := s.p8RollGather(cfg)
			if err != nil {
				return err
			}
			old = db.P8GatherAttempt{PlayerID: id, Token: token, Scene: node.Scene, PosID: node.PosID, Generation: node.Generation, StartedAt: now, ReadyAt: now + cfg.GatherMS, Active: true, Snapshot: node.Snapshot, Rewards: rewards}
			if err = tx.Save(&old).Error; err != nil {
				return err
			}
		}
		if cfg.Kind == "fish" {
			g.Field.Side.State = proto.Int32(constUtil.P8FieldFishing)
		} else {
			g.Field.Side.State = proto.Int32(constUtil.P8FieldMining)
		}
		out.Result = proto.Int32(constUtil.P8ResourceAvailable)
		out.Need = []*pb.BaseResource{{Id: proto.String("tiredness"), Num: proto.Int32(cfg.Fatigue)}}
		return nil
	})
}

func (s *GameService) p8ClaimGather(ctx context.Context, id int64, b *pb.CSFieldGetResource, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFieldGetResource{}
	return s.p8GatherMutation(ctx, id, constUtil.P8GatherClaimOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		var attempt db.P8GatherAttempt
		if err := tx.First(&attempt, "player_id=? AND active=?", id, true).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			out.Code = proto.Int32(constUtil.P8ResourceMissing)
			return nil
		} else if err != nil {
			return err
		}
		cfg, err := p8DecodeConfig(attempt.Snapshot)
		if err != nil {
			return err
		}
		if now < attempt.ReadyAt || attempt.Scene != g.Position.GetScene() || !p8Near(g.Position.GetCurrent(), cfg) || g.Field.Side.GetTiredness() < cfg.Fatigue {
			return p8GatherError()
		}
		var node db.P8ResourceNode
		if err = tx.First(&node, "player_id=? AND scene=? AND pos_id=?", id, attempt.Scene, attempt.PosID).Error; err != nil {
			return err
		}
		if node.Generation != attempt.Generation || node.Remaining < 1 || node.ResourceID != cfg.ResourceID {
			return p8GatherError()
		}
		g.Field.Side.Tiredness = proto.Int32(g.Field.Side.GetTiredness() - cfg.Fatigue)
		gain, err := s.p8GrantGather(p, g, attempt.Rewards)
		if err != nil {
			return err
		}
		node.Remaining--
		if node.Remaining == 0 {
			node.RefreshAt = now + cfg.CooldownMS
		}
		if err = tx.Save(&node).Error; err != nil {
			return err
		}
		attempt.Active = false
		if err = tx.Save(&attempt).Error; err != nil {
			return err
		}
		g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
		kind := constUtil.P8EventMineGather
		if cfg.Kind == "fish" {
			kind = constUtil.P8EventFishGather
		}
		if err = s.RecordP1Event(tx, id, "p8:gather:"+attempt.Token, kind, 1, time.UnixMilli(now)); err != nil {
			return err
		}
		out.Code = proto.Int32(constUtil.P8ResourceAvailable)
		out.Cost = []*pb.BaseResource{{Id: proto.String("tiredness"), Num: proto.Int32(cfg.Fatigue)}}
		out.Res = gain
		return nil
	})
}

func (s *GameService) p8ResourceQuery(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var out proto.Message
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		if err := s.p8GatherOpen(id, p, g); err != nil {
			return err
		}
		nodes, _, err := s.p8Nodes(tx, id, g.Position.GetScene(), time.Now().UnixMilli())
		if err != nil {
			return err
		}
		var posID string
		switch b := request.(type) {
		case *pb.CSSideResourceCheck:
			posID = b.GetPosId()
		case *pb.CSGatherCost:
			posID = b.GetPosId()
		default:
			return p8GatherError()
		}
		var node *db.P8ResourceNode
		for i := range nodes {
			if nodes[i].PosID == posID {
				node = &nodes[i]
			}
		}
		switch request.(type) {
		case *pb.CSSideResourceCheck:
			result := &pb.SCSideResourceCheck{PosId: proto.String(posID), Code: proto.Int32(constUtil.P8ResourceMissing)}
			if node != nil {
				result.ResourceId = proto.String(node.ResourceID)
				if node.Remaining > 0 {
					result.Code = proto.Int32(constUtil.P8ResourceAvailable)
				}
			}
			out = result
		case *pb.CSGatherCost:
			if node == nil || node.Remaining == 0 {
				return p8GatherError()
			}
			cfg, err := p8DecodeConfig(node.Snapshot)
			if err != nil {
				return err
			}
			out = &pb.SCGatherCost{Need: []*pb.BaseResource{{Id: proto.String("tiredness"), Num: proto.Int32(cfg.Fatigue)}}}
		}
		return saveProfile(tx, row, p, g)
	})
	return []proto.Message{out}, err
}

// DispatchP8Gather handles the original six maps; multiplayer ownership remains
// the explicit later domain rather than changing these isolated player nodes.
func (s *GameService) DispatchP8Gather(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var out []proto.Message
	var err error
	switch b := request.(type) {
	case *pb.CSGuideFish:
		out, err = s.p8GuideResource(ctx, id, b, stamp, true)
	case *pb.CSGuideMine:
		out, err = s.p8GuideResource(ctx, id, b, stamp, false)
	case *pb.CSFieldResourceStartGather:
		out, err = s.p8StartGather(ctx, id, b, stamp)
	case *pb.CSFieldGetResource:
		out, err = s.p8ClaimGather(ctx, id, b, stamp)
	case *pb.CSSideResourceCheck, *pb.CSGatherCost:
		out, err = s.p8ResourceQuery(ctx, id, request)
	default:
		return nil, false, nil
	}
	return out, true, err
}

// P8GatherNodes 为隔离验收和运维返回稳定排序，不产生客户端自选奖励。
func P8GatherNodes(tx *gorm.DB, id int64) ([]db.P8ResourceNode, error) {
	var rows []db.P8ResourceNode
	err := tx.Where("player_id=?", id).Find(&rows).Error
	sort.Slice(rows, func(i, j int) bool { return rows[i].PosID < rows[j].PosID })
	return rows, err
}
