package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP7ReRush 创建独立来源和区段账本；实际目录切换由发布步骤显式调用。
func (s *GameService) InitializeP7ReRush(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7ReRushPolicy{}, &db.P7ReRushSection{}, &db.P7ReRushSweep{}, &db.P7ReRushRun{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7ReRushPolicy{ID: 1, Source: constUtil.P7ReRushSource}).Error
}

// PublishP7ReRush 原子发布 Re 全目录，历史战斗/奖励/轮次保留其冻结快照。
// 此函数不会打开全局权威开关，也不会补写任何玩家通关或发放资源。
func (s *GameService) PublishP7ReRush(ctx context.Context) error {
	rows, err := s.p7ReRushCatalogue(true)
	if err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := tx.Model(&db.P7RushStageRule{}).Where("kind=? AND source<>?", constUtil.P7RushTrial, constUtil.P7ReRushSource).Update("enabled", false).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, 100).Error; err != nil {
			return err
		}
		return tx.Save(&db.P7ReRushPolicy{ID: 1, Enabled: true, Source: constUtil.P7ReRushSource}).Error
	})
}

func p7ReRushEnabled(tx *gorm.DB) (bool, error) {
	if !tx.Migrator().HasTable(&db.P7ReRushPolicy{}) {
		return false, nil
	}
	var policy db.P7ReRushPolicy
	err := tx.First(&policy, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return policy.Enabled && policy.Source == constUtil.P7ReRushSource, err
}

func p7ReRushTaskProjection(stage db.P7RushStageRule) *pb.RushTaskInfo {
	return &pb.RushTaskInfo{TaskId: proto.String(stage.TaskID), Star: proto.Int32(stage.Star), FightScore: proto.Float64(stage.RecommendedPower), IsPassed: proto.Bool(false), Coin: proto.Int32((2000 + stage.Floor*500) * stage.Star), Point: proto.Int32((50 + stage.Floor*10) * stage.Star)}
}

func p7ReRushNavigation(floor int32) string {
	floor = max(1, min(floor, constUtil.P7ReRushLastFloor))
	return fmt.Sprintf("map_portal_%03d", 1+((floor-1)/21)%5)
}

// p7ReRushProperties 适配 Re 的分段指数插值和三星难度，不改动原引擎公式。
// 锚点来自 OfflineRushBalance316 的 FieldRVA 数组，保留其 Math.Round 偶数舍入。
func p7ReRushProperties(floor, star int32, elite bool) (*pb.BaseProp, error) {
	if floor < 1 || floor > constUtil.P7ReRushLastFloor || star < 1 || star > 3 {
		return nil, errors.New("Re试炼楼层或难度无效")
	}
	floors := []int32{1, 20, 40, 60, 80, 90, 102}
	at := func(values []float64) float64 {
		for i := 1; i < len(floors); i++ {
			if floor <= floors[i] {
				t := float64(floor-floors[i-1]) / float64(floors[i]-floors[i-1])
				return math.Exp(math.Log(values[i-1])*(1-t) + math.Log(values[i])*t)
			}
		}
		return values[len(values)-1]
	}
	factor := []float64{1, 1.25, 1.55}[star-1]
	hp := at([]float64{3500, 60000, 450000, 1800000, 4800000, 7600000, 12000000}) * factor
	att := at([]float64{220, 4000, 32000, 125000, 350000, 560000, 900000}) * factor
	def := at([]float64{50, 700, 7000, 28000, 85000, 145000, 240000}) * factor
	if elite {
		hp *= 3
		att *= 1.25
		def *= 1.1
	}
	return &pb.BaseProp{Hp: proto.Float64(math.RoundToEven(hp)), Att: proto.Float64(math.RoundToEven(att)), Pdef: proto.Float64(math.RoundToEven(def)), Mdef: proto.Float64(math.RoundToEven(def)), Hit: proto.Float64(110 + 10*float64(floor-1)/101), Dodge: proto.Float64(0), Cri: proto.Float64(5 + 10*float64(floor-1)/101), AntiCri: proto.Float64(5 + 15*float64(floor-1)/101), CriFactor: proto.Float64(50), DamageAdd: proto.Float64(0), DamageReduce: proto.Float64(0)}, nil
}

// p7ReRushEnemy 从 Re 固定阵容和当前原 Hero 表创建完整协议实例。
// 精英头像映射仅用于明确采用 Re 规则的来源，旧原服目录继续保留原证据标记。
func (s *GameService) p7ReRushEnemy(floor, star int32, eliteID string) (*pb.BattleMonster, error) {
	prop, err := p7ReRushProperties(floor, star, eliteID != "")
	if err != nil {
		return nil, err
	}
	roster := []string{"monster_044", "monster_033", "hero_005", "hero_019", "hero_024"}
	positions := []int32{3, 7, 8, 9, 13}
	count := min(5, 2+int(floor-1)/20)
	if eliteID != "" {
		count = 1
		row := s.table("RushEliteBoss", "EliteTaskId", eliteID)
		if row == nil {
			return nil, errors.New("Re试炼精英原表缺失")
		}
		id := strings.Replace(row["IconName"], "hero_icon_", "hero_", 1)
		if hero := s.table("Hero", "id", id); hero != nil && hero["resName"] != "" {
			roster[0] = id
		}
	}
	monster := &pb.BattleMonster{MonsterId: proto.Int64(910000 + int64(floor)), Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	for i := 0; i < count; i++ {
		row := s.table("Hero", "id", roster[i])
		if row == nil || row["resName"] == "" || row["SkillId1"] == "" || s.table("Skill", "SkillId", row["SkillId1"]) == nil {
			return nil, fmt.Errorf("Re试炼固定阵容原资源缺失：%s", roster[i])
		}
		blood := int32(1)
		if eliteID != "" {
			blood = 3
		}
		monster.Formation = append(monster.Formation, &pb.BattleHero{Id: proto.String(roster[i]), Guid: proto.String(fmt.Sprintf("rush-%d-%d", floor, i)), Level: proto.Int32(min(100, 22+(floor-1)*78/101)), Pose: proto.Int32(positions[i]), BloodNumber: proto.Int32(blood), SkillOrder: proto.String(strings.Join([]string{row["SkillId1"], row["SkillId2"], row["UltSkillId"]}, ",")), SkillCastChance: proto.Float64(0.35), AwakenSkill: proto.String(""), WeaponId: proto.String(""), Prop: proto.Clone(prop).(*pb.BaseProp), Inst: &pb.BattleHeroInst{CurHp: proto.Float64(prop.GetHp())}})
	}
	return monster, nil
}

// p7ReRushCatalogue 批量生成102层三难度与原表全部精英，调用者负责事务发布。
// 奖励保留 Re 的层数公式及单参 Amount 的原样返回行为，初始化不发奖。
func (s *GameService) p7ReRushCatalogue(enabled bool) ([]db.P7RushStageRule, error) {
	maps := []string{"map_maze_001", "map_cave_001", "map_forest_002", "map_snowforest_001", "map_desert_001"}
	var rows []db.P7RushStageRule
	makeRow := func(floor, star int32, eliteID string) (db.P7RushStageRule, error) {
		monster, err := s.p7ReRushEnemy(floor, star, eliteID)
		if err != nil {
			return db.P7RushStageRule{}, err
		}
		enemy, err := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
		if err != nil {
			return db.P7RushStageRule{}, err
		}
		prop := monster.Formation[0].Prop
		power := math.RoundToEven((prop.GetHp()*.25 + prop.GetAtt()*6 + prop.GetPdef()*10 + prop.GetMdef()*10) * .1 * float64(min(5, 2+(floor-1)/20)))
		// 当前轮 Floor 仅由实际结算/合法扫荡推进；保留既有来源玩家当前楼层。
		row := db.P7RushStageRule{ID: fmt.Sprintf("re-rush:%d:%d", floor, star), Kind: constUtil.P7RushTrial, TaskID: fmt.Sprintf("local_rush_%d", floor), Floor: floor, Star: star, MinLevel: 22, Enabled: enabled, Scene: maps[((floor-1)/21)%5], MaxRound: 40, Enemies: enemy, Reward: fmt.Sprintf("coin,%d;point,%d", (2000+floor*500)*star, (50+floor*10)*star), RecommendedPower: power, Source: constUtil.P7ReRushSource}
		if eliteID != "" {
			original := s.table("RushEliteBoss", "EliteTaskId", eliteID)
			row.ID, row.Kind, row.TaskID, row.Star = "elite:"+eliteID, constUtil.P7RushElite, eliteID, 0
			row.ConditionTaskID, row.FirstReward, row.Reward = original["ConditionTaskId"], original["FirstReward"], original["Reward"]
		}
		return row, nil
	}
	for floor := int32(1); floor <= constUtil.P7ReRushLastFloor; floor++ {
		for star := int32(1); star <= 3; star++ {
			row, err := makeRow(floor, star, "")
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}
	for _, original := range s.Tables["RushEliteBoss"] {
		floor, err := p7Integer(original["ConditionTaskId"], 1, constUtil.P7ReRushLastFloor)
		if err != nil {
			return nil, err
		}
		row, err := makeRow(floor, 1, original["EliteTaskId"])
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}
