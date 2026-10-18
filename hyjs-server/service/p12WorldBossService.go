package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
	"time"
)

type p12WorldBossDefinition struct {
	Tables   map[string][]map[string]string
	Settings map[string]string
	// 原UI支持num=-1隐藏数量；本地仅可能ID投影须先具名发布并由周期定义SHA冻结。
	WeightedRewardPreviewSource string `json:",omitempty"`
}

func (s *GameService) LoadP12WorldBossTables(root string) error {
	for _, name := range []string{"WorldBoss", "WorldBossFactor", "WorldBossPos", "WorldBossPropadd", "WorldBossReward"} {
		if err := s.loadP7Table(root, name); err != nil {
			return err
		}
	}
	return nil
}
func (s *GameService) InitializeP12WorldBoss(ctx context.Context) error {
	def := p12WorldBossDefinition{Tables: map[string][]map[string]string{}, Settings: map[string]string{}, WeightedRewardPreviewSource: constUtil.P12WorldBossIDPreviewSource}
	for _, name := range []string{"WorldBoss", "WorldBossFactor", "WorldBossPos", "WorldBossPropadd", "WorldBossReward"} {
		if len(s.Tables[name]) == 0 {
			return errors.New("原世界首领完整定义缺失")
		}
		def.Tables[name] = s.Tables[name]
	}
	for _, name := range []string{"worldbossRefreshTime1", "worldbossRefreshTime2", "worldBossTimeDurationMinutes", "WorldBossMaxBuyCount", "WorldBossPerBuyPrice", "WorldBossFreeTimes", "WorldBossRecoverTime", "worldbossMaxLevel", "worldbossLevelUpKillTimeMinutes", "worldbossNotKillCount"} {
		row := s.table("GameConfigure", "StaticName", name)
		if row == nil {
			return errors.New("原世界首领日历/次数配置缺失")
		}
		def.Settings[name] = row["NameValue"]
	}
	if _, _, _, err := p12WorldBossWindow(def, time.Now()); err != nil {
		return err
	}
	if _, _, err := p12WorldBossProjection(def, 1, 1); err != nil {
		return err
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err = tx.AutoMigrate(&db.P12WorldBossPolicy{}, &db.P12WorldBossCycle{}); err != nil {
		return err
	}
	if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P12WorldBossPolicy{ID: 1, Level: 1, PositionID: 1, DefinitionJSON: string(raw), DefinitionHash: p6Digest(raw), Source: constUtil.P12WorldBossSource}).Error; err != nil {
		return err
	}
	if err = s.InitializeP12WorldBossBattle(ctx); err != nil {
		return err
	}
	s.P12WorldBossBattleReady = true
	return nil
}
func p12WorldBossConfig(tx *gorm.DB) (*db.P12WorldBossPolicy, *p12WorldBossDefinition, error) {
	var cfg db.P12WorldBossPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
		return nil, nil, err
	}
	if cfg.Source == "" || cfg.Level < 1 || cfg.Level > 50 || cfg.PositionID < 1 || cfg.DefinitionHash != p6Digest([]byte(cfg.DefinitionJSON)) {
		return nil, nil, errors.New("世界首领完整运营快照无效")
	}
	var def p12WorldBossDefinition
	if err := json.Unmarshal([]byte(cfg.DefinitionJSON), &def); err != nil {
		return nil, nil, err
	}
	if s, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok {
		if err := s.adminFieldBossConfiguration(&cfg, &def); err != nil {
			return nil, nil, err
		}
	}
	return &cfg, &def, nil
}

// p12WorldBossWindow使用原两个刷新时刻/60分钟，边界为开始包含、结束不包含；返回当前或最近未来原窗口。
func p12WorldBossWindow(def p12WorldBossDefinition, now time.Time) (string, time.Time, time.Time, error) {
	if def.Settings["worldbossRefreshTime1"] != "12:00" || def.Settings["worldbossRefreshTime2"] != "21:40" || def.Settings["worldBossTimeDurationMinutes"] != "60" {
		return "", time.Time{}, time.Time{}, errors.New("原世界首领12:00/21:40/60分钟日历未完整")
	}
	location := time.FixedZone("UTC+8", 8*3600)
	local := now.In(location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for offset := 0; offset < 2; offset++ {
		for _, clock := range []string{def.Settings["worldbossRefreshTime1"], def.Settings["worldbossRefreshTime2"]} {
			parsed, err := time.Parse("15:04", clock)
			if err != nil {
				return "", time.Time{}, time.Time{}, err
			}
			start := day.AddDate(0, 0, offset).Add(time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute)
			end := start.Add(time.Hour)
			if now.Before(end) {
				return fmt.Sprintf("world:%s:%s", start.Format("2006-01-02"), clock), start, end, nil
			}
		}
	}
	return "", time.Time{}, time.Time{}, errors.New("原世界首领下一窗口不存在")
}
func p12WorldBossProjection(def p12WorldBossDefinition, level, positionID int64) (map[string]string, map[string]string, error) {
	var leader, pos map[string]string
	for _, row := range def.Tables["WorldBoss"] {
		if row["BossId"] == "boss_001" && row["show"] == "1" {
			if leader != nil {
				return nil, nil, errors.New("原世界首领展示成员重复")
			}
			leader = row
		}
	}
	for _, row := range def.Tables["WorldBossPos"] {
		if row["id"] == strconv.FormatInt(positionID, 10) && row["BossId"] == "boss_001" {
			pos = row
		}
	}
	factors := 0
	seen := map[string]bool{}
	for _, row := range def.Tables["WorldBossFactor"] {
		if row["BossId"] == "boss_001" && row["Level"] == strconv.FormatInt(level, 10) {
			if seen[row["Location"]] {
				return nil, nil, errors.New("原世界首领等级阵位重复")
			}
			seen[row["Location"]] = true
			factors++
		}
	}
	if leader == nil || pos == nil || pos["scene"] == "" || factors != 4 || len(def.Tables["WorldBoss"]) != 4 || len(def.Tables["WorldBossPropadd"]) != 4 {
		return nil, nil, errors.New("原世界首领完整四成员/等级/出生/增益未匹配")
	}
	return leader, pos, nil
}

// p12WorldBossPossibleIDs 只识别原id:quantity,weight选项是否有非零数量/权重，不计算概率或固定数量。
func p12WorldBossPossibleIDs(content string) ([]*pb.BaseResource, error) {
	var result []*pb.BaseResource
	seen := map[string]bool{}
	for _, part := range strings.Split(content, "|") {
		pair := strings.Split(part, ":")
		if len(pair) != 2 || pair[0] == "" || strings.TrimSpace(pair[0]) != pair[0] || strings.ContainsAny(pair[0], ",;\x00") || seen[pair[0]] {
			return nil, errors.New("原世界首领可能ID定义无效或重复")
		}
		for _, c := range pair[0] {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				return nil, errors.New("原世界首领奖池ID字符无效")
			}
		}
		seen[pair[0]] = true
		possible := false
		for _, alternative := range strings.Split(pair[1], ";") {
			option := strings.Split(alternative, ",")
			if len(option) != 2 {
				return nil, errors.New("原世界首领奖池选项格式无效")
			}
			quantity, quantityErr := strconv.ParseInt(option[0], 10, 32)
			weight, weightErr := strconv.ParseInt(option[1], 10, 32)
			if quantityErr != nil || weightErr != nil || quantity < 0 || weight < 0 {
				return nil, errors.New("原世界首领奖池选项数量/权重无效")
			}
			if quantity > 0 && weight > 0 {
				possible = true
			}
		}
		if possible {
			result = append(result, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(-1)})
		}
	}
	if len(result) == 0 {
		return nil, errors.New("原世界首领可能ID预览为空")
	}
	return result, nil
}

// 原OnWorldBossRewardCallback只刷新奖档展示；按实例冻结等级读取完整六档，不生成附件或领取事实。
func p12WorldBossRewardPreview(def p12WorldBossDefinition, level int64) (*pb.SCWorldBossReward, error) {
	rows := make([]*pb.WorldBossRewardInfo, 6)
	for _, row := range def.Tables["WorldBossReward"] {
		if row["level"] != strconv.FormatInt(level, 10) {
			continue
		}
		kind, err := strconv.ParseInt(row["type"], 10, 32)
		if err != nil || kind < 0 || kind >= int64(len(rows)) || rows[kind] != nil {
			return nil, errors.New("原世界首领奖励预览类型无效或重复")
		}
		var rewards []*pb.BaseResource
		if strings.ContainsAny(row["reward"], ":|") {
			// 只读解析可能出现的ID；-1沿原CommonIcon隐藏数量，不抽取、承诺或发放随机奖励。
			if level < 30 || level > 50 || (kind != 4 && kind != 5) || def.WeightedRewardPreviewSource != constUtil.P12WorldBossIDPreviewSource {
				return nil, errors.New("原世界首领可能ID预览缺少具名冻结来源")
			}
			rewards, err = p12WorldBossPossibleIDs(row["reward"])
		} else {
			rewards, err = p1RewardResources(row["reward"])
		}
		if err != nil || len(rewards) == 0 {
			return nil, errors.New("原世界首领奖励预览资源格式无效")
		}
		rows[kind] = &pb.WorldBossRewardInfo{Type: proto.Int32(int32(kind)), Rewards: rewards}
	}
	for _, row := range rows {
		if row == nil {
			return nil, errors.New("原世界首领完整六档奖励预览缺失")
		}
	}
	return &pb.SCWorldBossReward{RewardInfos: rows}, nil
}

// p12WorldBossCycle持运营互斥锁并以当前读恢复周期UUID，重启/并发不会把一个原窗口复制为多个首领。
func p12WorldBossCycle(tx *gorm.DB, cfg *db.P12WorldBossPolicy, def *p12WorldBossDefinition, now time.Time) (*db.P12WorldBossCycle, *p12WorldBossDefinition, error) {
	var previous []db.P12WorldBossCycle
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("state<>? AND ends_at<=?", constUtil.P12WorldBossEnded, now.UnixMilli()).Order("starts_at").Find(&previous).Error; err != nil {
		return nil, nil, err
	}
	for i := range previous {
		previous[i].State = constUtil.P12WorldBossEnded
		previous[i].EndedAt = previous[i].EndsAt
		if err := tx.Save(&previous[i]).Error; err != nil {
			return nil, nil, err
		}
	}
	key, start, end, err := p12WorldBossWindow(*def, now)
	if err != nil {
		return nil, nil, err
	}
	var cycle db.P12WorldBossCycle
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cycle, "window_key=?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cycle = db.P12WorldBossCycle{ID: uuid.NewString(), WindowKey: key, State: constUtil.P12WorldBossScheduled, StartsAt: start.UnixMilli(), EndsAt: end.UnixMilli(), DefinitionJSON: cfg.DefinitionJSON, DefinitionHash: cfg.DefinitionHash, Level: cfg.Level, PositionID: cfg.PositionID, CreatedAt: now.UnixMilli()}
		if err = tx.Create(&cycle).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	if cycle.StartsAt != start.UnixMilli() || cycle.EndsAt != end.UnixMilli() || cycle.DefinitionHash != p6Digest([]byte(cycle.DefinitionJSON)) {
		return nil, nil, errors.New("既有世界首领窗口/定义损坏")
	}
	if !now.Before(start) && cycle.State == constUtil.P12WorldBossScheduled {
		cycle.State = constUtil.P12WorldBossActive
		cycle.ActivatedAt = cycle.StartsAt
		if err = tx.Save(&cycle).Error; err != nil {
			return nil, nil, err
		}
	}
	var frozen p12WorldBossDefinition
	if err = json.Unmarshal([]byte(cycle.DefinitionJSON), &frozen); err != nil {
		return nil, nil, err
	}
	return &cycle, &frozen, nil
}
func (s *GameService) p12WorldBossAt(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	switch request.(type) {
	case *pb.CSWorldBossInfo, *pb.CSWorldBossRank, *pb.CSWorldBossReward:
	default:
		return nil, errors.New(constUtil.P12WorldBossClosed)
	}
	var reply proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.First(&profile, id).Error; err != nil {
			return err
		}
		p, _, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		if p.GetPlayerId() != id {
			return errors.New("世界首领查询角色归属无效")
		}
		cfg, def, err := p12WorldBossConfig(tx)
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			return errors.New(constUtil.P12WorldBossClosed)
		}
		cycle, frozen, err := p12WorldBossCycle(tx, cfg, def, now)
		if err != nil {
			return err
		}
		leader, pos, err := p12WorldBossProjection(*frozen, cycle.Level, cycle.PositionID)
		if err != nil {
			return err
		}
		state := int32(0)
		next := cycle.StartsAt
		if cycle.State == constUtil.P12WorldBossActive {
			state = 1
			_, nextStart, _, err := p12WorldBossWindow(*frozen, time.UnixMilli(cycle.EndsAt))
			if err != nil {
				return err
			}
			next = nextStart.UnixMilli()
		}
		switch request.(type) {
		case *pb.CSWorldBossInfo:
			out := &pb.WorldBossShowInfo{BossName: proto.String(leader["name"]), BossLevel: proto.Int32(int32(cycle.Level)), BossPicPath: proto.String(leader["HeroId"]), BossState: proto.Int32(state), LeftRefreshTime: proto.Int64(next), LvChange: proto.Int32(0), Scene: proto.String(pos["scene"]), Bossuuid: proto.String(cycle.ID), RewardShowEndTime: proto.Int64(0)}
			for _, row := range frozen.Tables["WorldBossPropadd"] {
				kind, err := strconv.ParseInt(row["type"], 10, 32)
				if err != nil || row["cost"] != "diamond,150" || (kind != 0 && kind != 1) {
					return errors.New("原世界首领完整增益类型/价格无效")
				}
				out.Props = append(out.Props, &pb.DictWorldBossPropertyAddition{Id: proto.String(row["id"]), Cost: proto.String(row["cost"]), Prop: proto.String(row["prop"]), Desp: proto.String(row["desp"]), Type: proto.Int32(int32(kind)), Icon: proto.String(row["icon"])})
			}
			if s.P12WorldBossBattleReady && cycle.State == constUtil.P12WorldBossActive {
				var battlePolicy db.P12WorldBossBattlePolicy
				if err := tx.First(&battlePolicy, 1).Error; err != nil {
					return err
				}
				if battlePolicy.Enabled {
					out.PropPurchased, err = p12WorldBossPurchasedIDs(tx, cycle.ID, id)
					if err != nil {
						return err
					}
					instance, err := s.p12WorldBossBattleInstanceLocked(tx, cycle, frozen)
					if err != nil {
						return err
					}
					if instance.CurrentHP == 0 {
						out.BossState = proto.Int32(2)
					}
				}
			}
			reply = &pb.SCWorldBossInfo{WorldBossShowInfo: out}
		case *pb.CSWorldBossRank:
			if s.P12WorldBossBattleReady {
				var battlePolicy db.P12WorldBossBattlePolicy
				if err := tx.First(&battlePolicy, 1).Error; err != nil {
					return err
				}
				if battlePolicy.Enabled && cycle.State == constUtil.P12WorldBossActive {
					reply, err = p12WorldBossBattleRank(tx, cycle.ID, id)
					return err
				}
			}
			// 本批没有权威攻击入口；未发生伤害的真实定义保持满条，列表/最后一击/个人排名均为空。
			reply = &pb.SCWorldBossRank{BossHpPercent: proto.Float64(1), State: proto.Int32(state)}
		case *pb.CSWorldBossReward:
			reply, err = p12WorldBossRewardPreview(*frozen, cycle.Level)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.P12WorldBossBattleReady {
		if err = s.SyncP12WorldBossRewards(ctx, id); err != nil {
			return nil, err
		}
	}
	return []proto.Message{reply}, nil
}
func (s *GameService) DispatchP12WorldBoss(ctx context.Context, id int64, request proto.Message, timestamps ...int64) ([]proto.Message, bool, error) {
	if purchase, ok := request.(*pb.CSWorldBossPropPurchase); ok {
		response, err := s.PurchaseP12WorldBossProp(ctx, id, purchase.GetPropId(), LocalPlayNow(ctx))
		if err != nil {
			return nil, true, err
		}
		return []proto.Message{response}, true, nil
	}
	if start, ok := request.(*pb.CSWorldBossFight); ok {
		stamp := int64(0)
		if len(timestamps) != 0 {
			stamp = timestamps[0]
		}
		reply, err := s.StartP12WorldBossBattle(ctx, id, start.GetBossId(), stamp)
		if err != nil {
			return nil, true, err
		}
		return []proto.Message{reply}, true, nil
	}
	switch request.(type) {
	case *pb.CSWorldBossInfo, *pb.CSWorldBossRank, *pb.CSWorldBossPropPurchase, *pb.CSWorldBossFight, *pb.CSWorldBossReward:
	default:
		return nil, false, nil
	}
	out, err := s.p12WorldBossAt(ctx, id, request, LocalPlayNow(ctx))
	return out, true, err
}

// p12WorldBossFieldProjection 用当前已提交共享血量刷新原野外实体；预计算不会出现在这里。
func (s *GameService) p12WorldBossFieldProjection(tx *gorm.DB, scene string, now time.Time) ([]proto.Message, error) {
	if !s.P12WorldBossBattleReady {
		return nil, nil
	}
	var battlePolicy db.P12WorldBossBattlePolicy
	if err := tx.First(&battlePolicy, 1).Error; err != nil {
		return nil, err
	}
	if !battlePolicy.Enabled {
		return nil, nil
	}
	cfg, def, err := p12WorldBossConfig(tx)
	if err != nil || !cfg.Enabled {
		return nil, err
	}
	cycle, frozen, err := p12WorldBossCycle(tx, cfg, def, now)
	if err != nil {
		return nil, err
	}
	if cycle.State != constUtil.P12WorldBossActive {
		// 原周期结束后按旧UUID撤下实体，不让下一次刷新前仍可点击过期首领。
		var previous db.P12WorldBossCycle
		err := tx.Where("state=?", constUtil.P12WorldBossEnded).Order("ends_at DESC").First(&previous).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var instance db.P12WorldBossBattleInstance
		if err := tx.First(&instance, "id=?", previous.ID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		entity, err := p12WorldBossBattleEntity(tx, &previous)
		if err != nil || entity.GetScene() != scene {
			return nil, err
		}
		entity.State = proto.Int32(3)
		return []proto.Message{&pb.SCWorldBoss{WorldBoss: entity}}, nil
	}
	_, position, err := p12WorldBossProjection(*frozen, cycle.Level, cycle.PositionID)
	if err != nil || position["scene"] != scene {
		return nil, err
	}
	if _, err = s.p12WorldBossBattleInstanceLocked(tx, cycle, frozen); err != nil {
		return nil, err
	}
	entity, err := p12WorldBossBattleEntity(tx, cycle)
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCWorldBoss{WorldBoss: entity}}, nil
}
