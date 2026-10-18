package service

import (
	"math"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The isolated table fixture supplies the exact original native four-property
// snapshots. It tests the real shared party calculation and source pet bonus,
// including SDK Round(double)'s half-to-even rule and rounding after summation.
func TestP9NativePowerRounding(t *testing.T) {
	fixture := func(t *testing.T, a, b map[string]float64, guard bool) (*GameService, *pb.Player, *model.GameState) {
		t.Helper()
		s := &GameService{}
		if err := s.LoadTables("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
		if err := s.LoadP9Tables("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
		p := &pb.Player{PlayerId: proto.Int64(999), Level: proto.Int32(1), HeroWarId: proto.String("power-main")}
		g := &model.GameState{}
		for index, entry := range []struct {
			id, guid string
			props    map[string]float64
		}{{"hero_001", "power-main", a}, {"hero_003", "power-asuna", b}} {
			if entry.props == nil {
				continue
			}
			row := s.table("Hero", "id", entry.id)
			for _, key := range []string{"att", "hp", "pdef", "mdef"} {
				row[key+"Base"] = strconv.FormatFloat(entry.props[key], 'f', -1, 64)
			}
			for _, node := range s.Tables["TalentAdvenced"] {
				if node["warId"] == entry.id && node["talentLevel"] == "0" {
					for _, key := range []string{"att", "hp", "pdef", "mdef"} {
						node[key+"ConstAdd"] = "0"
					}
					node["talentId"] = ""
				}
			}
			w := &pb.Warrior{Id: proto.String(entry.id), Guid: proto.String(entry.guid), Level: proto.Int32(1), Exp: proto.Int64(0), Talent: proto.Int32(0), Training: &pb.Training{}, Breached: &pb.Breached{Level: proto.Int32(1)}, Awake: &pb.Awake{}}
			g.Warriors = append(g.Warriors, w)
			g.Formation = append(g.Formation, &pb.FormationItem{FormatPos: proto.Int32(int32(index + 1)), BattlePos: proto.Int32(int32(3 + index*4)), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}})
		}
		if guard {
			pet := &pb.Pet{Id: proto.String("pet_002"), Guid: proto.String("power-pet"), Level: proto.Int32(1), Exp: proto.Int64(0), InitProp: proto.String("PetTalent_11"), Eat: &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0), HdTimeList: []int32{0, 0, 0}}, Breached: &pb.PetBreached{Level: proto.Int32(0)}}
			if err := p9ProjectPet(g, pet); err != nil {
				t.Fatal(err)
			}
			g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Id: pet.Id, Guid: pet.Guid, Pos: proto.Int32(7)})
		}
		return s, p, g
	}
	t.Run("original-native1833-to1842-guard-positive-nine", func(t *testing.T) {
		main := map[string]float64{"att": 792, "hp": 13801, "pdef": 318, "mdef": 302}
		asuna := map[string]float64{"att": 172, "hp": 3899, "pdef": 98, "mdef": 94}
		var powers []int32
		for _, guard := range []bool{false, true} {
			s, p, g := fixture(t, main, asuna, guard)
			party, err := s.battlePlayer(p, g)
			if err != nil {
				t.Fatal(err)
			}
			att := 792.0
			if guard {
				att = 807
			}
			if party.GetFormation()[0].GetProp().GetAtt() != att || party.GetFormation()[0].GetProp().GetHp() != 13801 {
				t.Fatal("native source fixture drift", party)
			}
			power, err := s.formationFightPower(p, g)
			if err != nil {
				t.Fatal(err)
			}
			powers = append(powers, power)
		}
		if powers[0] != 1833 || powers[1] != 1842 || powers[1]-powers[0] != 9 {
			t.Fatalf("original native power %v", powers)
		}
	})
	t.Run("original-Math-Round-half-to-even-and-party-sum", func(t *testing.T) {
		for _, test := range []struct {
			a, b float64
			want int32
		}{{20, 0, 0}, {60, 0, 2}, {100, 0, 2}, {20, 40, 2}, {100, 40, 4}} {
			a := map[string]float64{"hp": test.a}
			var b map[string]float64
			if test.b > 0 {
				b = map[string]float64{"hp": test.b}
			}
			s, p, g := fixture(t, a, b, false)
			got, err := s.formationFightPower(p, g)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("SDK Math.Round(%v)=%d, got%d", (test.a+test.b)*0.025, test.want, got)
			}
			if float64(test.want) != math.RoundToEven((test.a+test.b)*0.025) {
				t.Fatal("native expected number drift")
			}
		}
	})
}
