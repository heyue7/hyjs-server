package service

import (
	"context"
	"errors"
	"sort"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p10FormationProfile 只投影真实持有且正在阵容的实例，不返回GUID/存档/会话秘密。
func (s *GameService) p10FormationProfile(tx *gorm.DB, id int64) (*pb.FormationSimpleInfo, error) {
	var row db.Profile
	if err := tx.First(&row, id).Error; err != nil {
		return nil, err
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	if err = s.PrepareP5GuildPrayerProperties(tx.Statement.Context, tx, p, g, p13UFNow(tx.Statement.Context)); err != nil {
		return nil, err
	}
	actor, err := s.battlePlayer(p, g)
	if err != nil {
		return nil, err
	}
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return nil, err
	}
	result := &pb.FormationSimpleInfo{Name: p.Name, FightPower: proto.Int32(power)}
	actors := map[string]*pb.BattleHero{}
	for _, hero := range actor.GetFormation() {
		actors[hero.GetGuid()] = hero
	}
	for _, slot := range g.Formation {
		heroThing := formationHero(slot)
		if heroThing == nil {
			continue
		}
		hero := actors[heroThing.GetGuid()]
		if hero == nil || hero.GetId() != heroThing.GetId() {
			return nil, errors.New("角色公开阵容实例不一致")
		}
		warrior, err := s.formationWarrior(g, heroThing.GetGuid(), heroThing.GetId())
		if err != nil {
			return nil, err
		}
		item := &pb.FormationItemSimpleInfo{FormatPos: slot.FormatPos, WarInfo: &pb.WarriorSimpleInfo{Pos: proto.Int32(0), HeroId: hero.Id, Level: hero.Level, BreachedLevel: hero.BreachedLevel, BaseProp: proto.Clone(hero.GetProp()).(*pb.BaseProp), Talent: warrior.Talent, AwakeStage: proto.Int32(warrior.GetAwake().GetStage()), FuseLevel: proto.Int32(warrior.GetAwake().GetFuseLevel())}}
		for _, thing := range slot.GetThings() {
			switch pos := thing.GetPos(); {
			case pos == 0:
			case pos >= 1 && pos <= 4:
				owned, err := s.equipmentRecord(g, thing.GetGuid())
				if err != nil {
					return nil, err
				}
				if owned.GetEquipId() != thing.GetId() {
					return nil, errors.New("公开装备归属无效")
				}
				item.EquipInfoList = append(item.EquipInfoList, &pb.EquipmentSimpleInfo{Pos: thing.Pos, EquipId: owned.EquipId, Level: owned.Level, RefineLv: owned.RefineLv, Star: owned.Star, Gems: append([]string(nil), owned.GetGems()...), ForgeLevel: owned.ForgeLevel})
			case pos == 5 || pos == 6:
				owned, err := s.treasureRecord(g, thing.GetGuid())
				if err != nil {
					return nil, err
				}
				if owned.GetTreasureId() != thing.GetId() {
					return nil, errors.New("公开宝物归属无效")
				}
				item.TreaInfoList = append(item.TreaInfoList, &pb.TreasureSimpleInfo{Pos: thing.Pos, TreasureId: owned.TreasureId, StrengthLevel: owned.StrengthLevel, RefineLevel: owned.RefineLevel})
			case pos == 7:
				if !gameSchemaHasTable(tx, &db.P9PetInstance{}) {
					return nil, errors.New("公开宠物实例域尚未初始化")
				}
				var owned db.P9PetInstance
				if err := tx.Where("guid=? AND player_id=?", thing.GetGuid(), id).First(&owned).Error; err != nil {
					return nil, errors.New("公开宠物归属无效")
				}
				pet := &pb.Pet{}
				if proto.Unmarshal(owned.Data, pet) != nil || pet.GetId() != thing.GetId() || pet.GetGuid() != thing.GetGuid() {
					return nil, errors.New("公开宠物实例无效")
				}
				item.PetInfo = &pb.PetSimpleInfo{Pos: thing.Pos, PetId: pet.Id}
			default:
				return nil, errors.New("公开阵容位置尚未恢复")
			}
		}
		sort.Slice(item.EquipInfoList, func(i, j int) bool { return item.EquipInfoList[i].GetPos() < item.EquipInfoList[j].GetPos() })
		sort.Slice(item.TreaInfoList, func(i, j int) bool { return item.TreaInfoList[i].GetPos() < item.TreaInfoList[j].GetPos() })
		result.FormationItemInfoList = append(result.FormationItemInfoList, item)
	}
	sort.Slice(result.FormationItemInfoList, func(i, j int) bool {
		return result.FormationItemInfoList[i].GetFormatPos() < result.FormationItemInfoList[j].GetFormatPos()
	})
	return result, nil
}
func (s *GameService) DispatchP10Profile(ctx context.Context, id int64, request proto.Message) ([]proto.Message, bool, error) {
	b, ok := request.(*pb.CSFindPlayerInfo)
	if !ok {
		return nil, false, nil
	}
	if b.GetPlayerId() < 1 || b.GetServerId() != 0 && int64(b.GetServerId()) != s.OriginalServerID() {
		return nil, true, errors.New("仅恢复本服真实角色资料")
	}
	var info *pb.FormationSimpleInfo
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, err := p10LockProfiles(tx, id, b.GetPlayerId()); err != nil {
			return err
		}
		var cfg db.P10SocialPolicy
		if err := tx.First(&cfg, constUtil.P10SocialPolicyID).Error; err != nil {
			return err
		}
		// 资料为只读真实状态，新关系关闭不阻止原查询。
		blocked, err := p10Blocked(tx, id, b.GetPlayerId())
		if err != nil {
			return err
		}
		if blocked {
			return errors.New("黑名单阻止角色资料交互")
		}
		info, err = s.p10FormationProfile(tx, b.GetPlayerId())
		return err
	})
	if err != nil {
		return nil, true, err
	}
	return []proto.Message{&pb.SCFindPlayerInfo{FormationInfo: info}}, true, nil
}
