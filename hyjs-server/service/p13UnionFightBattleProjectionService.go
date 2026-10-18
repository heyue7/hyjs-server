package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// ProjectP13UnionFightFieldBattle 冻结当前原战场pose生命和实际战鼓来源；审计计算不能据此提前结算战争。
func (s *GameService) ProjectP13UnionFightFieldBattle(ctx context.Context, id int64, now time.Time) (*pb.BattlePlayer, map[string][]string, error) {
	var battle *pb.BattlePlayer
	var applied map[string][]string
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var err error
		battle, applied, err = s.p13UFFieldBattle(tx, id, now)
		return err
	})
	return battle, applied, err
}

// p13UFFieldBattle 允许调用方在同一事务Capture/Compute使用准确的当前HP和已冻结范围来源。
func (s *GameService) p13UFFieldBattle(tx *gorm.DB, id int64, now time.Time) (*pb.BattlePlayer, map[string][]string, error) {
	battle, applied, err := s.p13UFDrumBattle(tx, id, now)
	if err != nil {
		return nil, nil, err
	}
	actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
	if err != nil {
		return nil, nil, err
	}
	fight := frame.GetFightInfo()
	if fight == nil || fight.GetGuid() != id || len(fight.Index) != len(battle.Formation) || len(fight.HpPos) != len(fight.Index) || len(fight.MaxHpPos) != len(fight.Index) {
		return nil, nil, errors.New("原战场FightInfo pose/生命长度不符")
	}
	poses := map[int32]int{}
	for i, pose := range fight.Index {
		if _, seen := poses[pose]; seen {
			return nil, nil, errors.New("原战场生命pose重复")
		}
		poses[pose] = i
	}
	seen := map[string]bool{}
	seenPoses := map[int32]bool{}
	alive := false
	for _, hero := range battle.Formation {
		if hero == nil || hero.Prop == nil || hero.Inst == nil || hero.GetGuid() == "" || seen[hero.GetGuid()] || seenPoses[hero.GetPose()] {
			return nil, nil, errors.New("原战场冻结Formation生命身份损坏")
		}
		seen[hero.GetGuid()] = true
		seenPoses[hero.GetPose()] = true
		i, ok := poses[hero.GetPose()]
		if !ok {
			return nil, nil, errors.New("原战场生命pose与冻结阵容不符")
		}
		maximum, current := fight.MaxHpPos[i], fight.HpPos[i]
		if maximum <= 0 || maximum > 1e15 || math.IsNaN(maximum) || math.IsInf(maximum, 0) || current < 0 || current > maximum || math.IsNaN(current) || math.IsInf(current, 0) || math.Abs(maximum-hero.Prop.GetHp()) > max(1e-9, math.Abs(maximum)*1e-12) {
			return nil, nil, errors.New("原战场当前生命/最大生命与冻结Prop.Hp不符")
		}
		hero.Inst.CurHp = proto.Float64(current)
		alive = alive || current > 0
		applied[hero.GetGuid()] = append(applied[hero.GetGuid()], constUtil.P13UnionFightBattleProjectionSource+";match="+match.ID+";frame="+actor.FrameHash+";battle="+actor.BattleHash+";pose="+strconv.Itoa(int(hero.GetPose())))
	}
	if !alive {
		return nil, nil, errors.New("原战场全阵容零生命不能冻结为存活参战者")
	}
	return battle, applied, nil
}
