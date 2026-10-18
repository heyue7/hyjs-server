package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
)

func (s *GameService) lightenWorldSeed(ctx context.Context, id int64, b *pb.CSLightenWorldSeed, stamp int64) ([]proto.Message, error) {
	response := &pb.SCLightenWorldSeed{}
	return s.p5Mutation(ctx, id, "world-seed", stamp, b, response, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if b.GetId() != p.GetCurrentWorldSeedId()+1 {
			return p5Error()
		}
		var row map[string]string
		for _, r := range s.Tables["WorldSeed"] {
			if p5SeedID(r) == b.GetId() {
				row = r
				break
			}
		}
		if row == nil {
			return p5Error()
		}
		costs, err := p3Resources(row["cost"])
		if err != nil || len(costs) == 0 {
			return p5Error()
		}
		for _, cost := range costs {
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
		}
		switch row["type"] {
		case "1":
		case "2":
			gain, err := s.GrantFixedReward(p, g, row["get"])
			if err != nil {
				return err
			}
			response.Get = gain
		case "3":
			main, err := p5Warrior(g, p.GetHeroWarId())
			if err != nil {
				return err
			}
			class := s.table("Hero", "id", main.GetId())["heroClass"]
			if class != "100" && class != "101" {
				return p5Error()
			}
			chosen := ""
			for _, candidate := range strings.Split(row["get"], ",") {
				if s.table("Hero", "id", candidate)["heroClass"] == class {
					if chosen != "" {
						return p5Error()
					}
					chosen = candidate
				}
			}
			if chosen == "" {
				return p5Error()
			}
			before := proto.Clone(main).(*pb.Warrior)
			main.Id = proto.String(chosen)
			main.SkillOrder = nil
			for _, slot := range g.Formation {
				for _, thing := range slot.Things {
					if thing.GetGuid() == main.GetGuid() {
						thing.Id = proto.String(chosen)
					}
				}
			}
			if err = s.syncEquipmentOwners(g); err != nil {
				return err
			}
			// 原品质转换保持同一成长实例，仅推进已有投入指纹。
			// 世界之种解锁成本属于全局成长，继续保留在全局消费中。
			if err = RecordP5Investment(g, main.GetGuid(), before, main, nil); err != nil {
				return err
			}
			response.NewWarrior = proto.Clone(main).(*pb.Warrior)
		default:
			return p5Error()
		}
		p.CurrentWorldSeedId = proto.Int32(b.GetId())
		response.Cost = costs
		return nil
	})
}

// SwapP5MainHero is a business API for the original 12-level swap. Native request transport
// is not present in the recovered protocol; root must expose only an audited client entry.
func (s *GameService) SwapP5MainHero(ctx context.Context, id int64, targetClass int32, stamp int64) ([]proto.Message, error) {
	b := &pb.CSSetSkillOrder{HeroGuid: proto.String("main-swap"), SkillOrder: proto.String(strconv.FormatInt(int64(targetClass), 10))}
	out := &pb.SCWarriorGet{}
	return s.p5Mutation(ctx, id, "main-swap", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		gate := int32(tableNumber(s.table("GameConfigure", "StaticName", "SwapMainHeroLevel"), "NameValue"))
		if gate <= 0 || p.GetLevel() < gate || (targetClass != 100 && targetClass != 101) {
			return p5Error()
		}
		w, err := p5Warrior(g, p.GetHeroWarId())
		if err != nil {
			return err
		}
		old := s.table("Hero", "id", w.GetId())
		if old == nil {
			return p5Error()
		}
		var target string
		for _, row := range s.Tables["Hero"] {
			if int32(tableNumber(row, "heroClass")) == targetClass && row["quality"] == old["quality"] {
				if target != "" {
					return p5Error()
				}
				target = row["id"]
			}
		}
		if target == "" {
			return p5Error()
		}
		before := proto.Clone(w).(*pb.Warrior)
		w.Id = proto.String(target)
		w.SkillOrder = nil
		for _, slot := range g.Formation {
			for _, thing := range slot.Things {
				if thing.GetGuid() == w.GetGuid() {
					thing.Id = proto.String(target)
				}
			}
		}
		if err = s.syncEquipmentOwners(g); err != nil {
			return err
		}
		if err = RecordP5Investment(g, w.GetGuid(), before, w, nil); err != nil {
			return err
		}
		out.Warrior = []*pb.Warrior{proto.Clone(w).(*pb.Warrior)}
		return nil
	})
}
