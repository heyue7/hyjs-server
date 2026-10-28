package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sort"
	"strings"
	"testing"
)

func TestP5GreenBlueFormation88OwnedWireAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	account, err := LoginAccount(f.ctx, "green-blue88-"+fmt.Sprint(f.id), "isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "绿蓝阵容验证", "hero_001"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	var ids []string
	green, blue := 0, 0
	for _, row := range f.s.Tables["Hero"] {
		q := int(tableNumber(row, "quality"))
		if strings.HasPrefix(row["id"], "hero_") && row["heroClass"] != "99" && (q == 12 || q == 13) {
			ids = append(ids, row["id"])
			if q == 12 {
				green++
			} else {
				blue++
			}
		}
	}
	sort.Strings(ids)
	if green != 6 || blue != 12 {
		t.Fatal("finite green/blue inventory changed", green, blue)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.WarriorBagSize = proto.Int32(1000)
		var rewards []string
		for _, id := range ids {
			rewards = append(rewards, id+",1")
		}
		_, e := f.s.GrantFixedReward(p, g, strings.Join(rewards, ";"))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	initial, game := f.profile(t)
	inventory, _ := json.Marshal(game.Warriors)
	cards := map[string]*pb.Warrior{}
	for _, w := range game.Warriors {
		cards[w.GetId()] = w
	}
	check := func(id string) {
		t.Helper()
		p, g := f.profile(t)
		var found bool
		for _, slot := range g.Formation {
			if slot.GetFormatPos() == 2 && formationHero(slot) != nil {
				found = formationHero(slot).GetGuid() == cards[id].GetGuid()
			}
		}
		if !found {
			t.Fatal("saved slot", id)
		}
		raw, _ := json.Marshal(g.Warriors)
		if string(raw) != string(inventory) || p.GetCoin() != initial.GetCoin() || p.GetDiamond() != initial.GetDiamond() {
			t.Fatal("on formation changed inventory or funds", id)
		}
		party, e := f.s.battlePlayer(p, g)
		if e != nil {
			t.Fatal("battle projection", id, e)
		}
		found = false
		for _, a := range party.Formation {
			if a.GetGuid() == cards[id].GetGuid() {
				found = true
				if strings.HasPrefix(id, "hero_05") || id == "hero_060" || id == "hero_061" {
					row := f.s.table("Hero", "id", id)
					if row["quality"] == "12" && (row["SkillId1"] == "" || !strings.Contains(a.GetSkillOrder(), row["SkillId1"])) {
						t.Fatal("green approved normal attack missing", id)
					}
				}
			}
		}
		if !found {
			t.Fatal("battle actor missing", id)
		}
	}
	for _, wire := range []bool{false, true} {
		for _, id := range ids {
			card := cards[id]
			if card == nil || f.s.P5FormationHeroAllowed(id) != nil {
				t.Fatal("owned eligibility", id)
			}
			request := &pb.CSFormationOn{Id: card.Id, Guid: card.Guid, FormatPos: proto.Int32(2)}
			if wire {
				out := copyOverflowWire(t, f, request)
				if _, ok := out[0].(*pb.SCFormationOn); !ok {
					t.Fatal("original popup on reply", id, out)
				}
			} else {
				if _, e := f.s.formationOn(f.ctx, f.id, request); e != nil {
					t.Fatal("legacy on", id, e)
				}
			}
			check(id)
			f.restart(t)
			check(id)
		}
	}
	// Experience cards, missing GUIDs, mismatches, duplicates, helpers and busy
	// dispatches remain rejected. The same original native route handles them.
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		for n, id := range []string{"hero_exp_001", "hero_exp_002"} {
			g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String(id), Guid: proto.String(fmt.Sprintf("material88-%d", n)), Level: proto.Int32(1)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reject := func(label string, b *pb.CSFormationOn) {
		t.Helper()
		p, g := f.profile(t)
		before, _ := json.Marshal(g)
		out := copyOverflowWire(t, f, b)
		if _, ok := out[0].(*pb.ErrorMessage); !ok {
			t.Fatal("invalid on accepted", label, out)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(p, afterP) || string(before) != string(after) {
			t.Fatal("rejected on changed state", label)
		}
	}
	for n, id := range []string{"hero_exp_001", "hero_exp_002"} {
		if f.s.P5FormationHeroAllowed(id) == nil {
			t.Fatal("material eligible")
		}
		reject(id, &pb.CSFormationOn{Id: proto.String(id), Guid: proto.String(fmt.Sprintf("material88-%d", n)), FormatPos: proto.Int32(2)})
	}
	card := cards["hero_056"]
	reject("missing", &pb.CSFormationOn{Id: card.Id, Guid: proto.String("foreign"), FormatPos: proto.Int32(2)})
	reject("mismatch", &pb.CSFormationOn{Id: cards["hero_032"].Id, Guid: card.Guid, FormatPos: proto.Int32(2)})
	// The current last actor cannot simultaneously occupy another slot.
	last := cards[ids[len(ids)-1]]
	reject("duplicate", &pb.CSFormationOn{Id: last.Id, Guid: last.Guid, FormatPos: proto.Int32(3)})
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.dispatch:"+card.GetGuid(), "owned-busy")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reject("busy", &pb.CSFormationOn{Id: card.Id, Guid: card.Guid, FormatPos: proto.Int32(2)})
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.dispatch:"+card.GetGuid(), "")
		p5SetKV(g, "p5.helper:1", card.GetGuid())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reject("helper", &pb.CSFormationOn{Id: card.Id, Guid: card.Guid, FormatPos: proto.Int32(2)})
	t.Log("6 green and12 blue: both formation entry points, original signed wire, cold reload and battle actors verified")
}
