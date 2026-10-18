package service

import (
	"context"
	"fmt"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Each colour has a finite 27-box/craft10 qualification. Original opening and
// thirteen actual three-combines supply the final gem without a gem injection.
func TestP8AllSixOriginalOrdinaryGemThirteenLifecycle(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	ctx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	read := func(id int64) (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if e := db.GetIns().First(&row, id).Error; e != nil {
			t.Fatal(e)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		return p, g
	}
	for colour := int32(1); colour <= 6; colour++ {
		t.Run(fmt.Sprint(colour), func(t *testing.T) {
			a, e := LoginAccount(f.ctx, fmt.Sprintf("originalordinary13-%d", colour), "isolated-test-only")
			if e != nil {
				t.Fatal(e)
			}
			if _, e = CreatePlayer(f.ctx, a.Id, "六色原十三宝石", "hero_043"); e != nil {
				t.Fatal(e)
			}
			if e = updateProfile(f.ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.EquipBagSize = proto.Int32(1000)
				_, e := f.s.GrantFixedReward(p, g, "item_345,27;item_119,1")
				return e
			}); e != nil {
				t.Fatal(e)
			}
			if e = db.GetIns().Create(&db.GemCraftState{PlayerID: a.Id, Level: 10}).Error; e != nil {
				t.Fatal(e)
			}
			open := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_345"), Num: proto.Int32(27)}, GetItemIndex: proto.Int32(colour)}
			if _, e = f.s.p5BoxUse(ctx(open), a.Id, open, 9001); e != nil {
				t.Fatal("actual27originalboxchoices", e)
			}
			var source, target string
			for _, row := range f.s.Tables["MineSkill"] {
				if int32(tableNumber(row, "Color")) == colour && row["Type"] == "magicGem" {
					if row["Level"] == "10" {
						source = row["MineItemID"]
					}
					if row["Level"] == "13" {
						target = row["MineItemID"]
					}
				}
			}
			_, g := read(a.Id)
			if gemCount(g, source) != 27 {
				t.Fatal("originalboxsource27", source, g.Gems)
			}
			stamp := int64(9100)
			var last proto.Message
			var lastRequest *pb.CSGemCombineByThree
			for level := 11; level <= 13; level++ {
				var current string
				for _, row := range f.s.Tables["MineSkill"] {
					if row["Type"] == "magicGem" && int32(tableNumber(row, "Color")) == colour && int(tableNumber(row, "Level")) == level {
						current = row["MineItemID"]
					}
				}
				count := map[int]int{11: 9, 12: 3, 13: 1}[level]
				for i := 0; i < count; i++ {
					stamp++
					lastRequest = &pb.CSGemCombineByThree{GemId: proto.String(current)}
					out, e := f.s.gemCombineThree(ctx(lastRequest), a.Id, lastRequest, stamp)
					if e != nil {
						t.Fatal("actualoriginalthreecombine", colour, level, e)
					}
					last = out[0]
				}
			}
			_, g = read(a.Id)
			if gemCount(g, target) != 1 || gemCount(g, source) != 0 {
				t.Fatal("27→9→3→1", g.Gems)
			}
			var gearChoice, hole int32
			for _, option := range f.s.Tables["ItemUse"] {
				if option["itemId"] != "item_119" {
					continue
				}
				// The original choice contains one canonical red equipment ID.
				var key string
				for _, row := range f.s.Tables["Equip"] {
					if option["result"] == row["id"]+",1" {
						key = row["id"]
						for pos := int32(0); pos < 4; pos++ {
							if int32(tableNumber(row, fmt.Sprintf("hole%d", pos+1))) == colour {
								gearChoice = int32(tableNumber(option, "odds"))
								hole = pos
								break
							}
						}
						break
					}
				}
				if key != "" && gearChoice > 0 {
					break
				}
			}
			if gearChoice == 0 {
				t.Fatal("no originalredhole", colour)
			}
			gearBox := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_119"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(gearChoice)}
			out, e := f.s.p5BoxUse(ctx(gearBox), a.Id, gearBox, 9200)
			if e != nil {
				t.Fatal(e)
			}
			gear := out[0].(*pb.SCItemUse).GetGet().GetEquips()[0]
			if _, e = f.s.formationOn(f.ctx, a.Id, &pb.CSFormationOn{Guid: gear.Guid, Id: gear.EquipId, FormatPos: proto.Int32(1)}); e != nil {
				t.Fatal(e)
			}
			p, g := read(a.Id)
			before, e := f.s.battlePlayer(p, g)
			if e != nil {
				t.Fatal(e)
			}
			mount := &pb.CSGemMount{Guid: gear.Guid, GemId: proto.String(target), Pos: proto.Int32(hole)}
			if _, e = f.s.gemMount(ctx(mount), a.Id, mount, 9300); e != nil {
				t.Fatal(e)
			}
			p, g = read(a.Id)
			after, e := f.s.battlePlayer(p, g)
			if e != nil {
				t.Fatal(e)
			}
			left, right := before.GetFormation()[0].GetProp(), after.GetFormation()[0].GetProp()
			deltas := []float64{right.GetAtt() - left.GetAtt(), right.GetHp() - left.GetHp(), right.GetPdef() - left.GetPdef(), right.GetMdef() - left.GetMdef(), right.GetCri() - left.GetCri(), right.GetAntiCri() - left.GetAntiCri()}
			expected := []float64{7000, 26600, 2100, 2100, 7.8, 7.8}
			for index, actual := range deltas {
				want := float64(0)
				if int32(index+1) == colour {
					want = expected[index]
				}
				if math.Abs(actual-want) > 1e-7 {
					t.Fatal("original13colour effect", colour, deltas)
				}
			}
			unmount := &pb.CSGemUnmount{Guid: gear.Guid, Pos: proto.Int32(hole)}
			if _, e = f.s.gemUnmount(ctx(unmount), a.Id, unmount, 9301); e != nil {
				t.Fatal(e)
			}
			f.restart(t)
			if again, e := f.s.gemCombineThree(ctx(lastRequest), a.Id, lastRequest, stamp); e != nil || !proto.Equal(again[0], last) {
				t.Fatal("latest simplehighest coldreceipt", e)
			}
			if _, e = f.s.gemMount(ctx(mount), a.Id, mount, 9300); e == nil {
				t.Fatal("oldmountafterunmount")
			}
			p, g = read(a.Id)
			restored, e := f.s.battlePlayer(p, g)
			if e != nil || !proto.Equal(restored.GetFormation()[0].GetProp(), left) || gemCount(g, target) != 1 || p.GetCoin() != 0 || p.GetDiamond() != 0 {
				t.Fatal("coldoriginalprops/gem/walletconservation", e)
			}
		})
	}
}
