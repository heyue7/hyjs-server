package service

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func TestP5BattleSkillTransport(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{"Hero": {{"id": "owned", "SkillId1": "a", "SkillId2": "b", "UltSkillId": "u", "AwakeSkillId": "c", "IsOpenAwake": "1"}}}}
	for _, selected := range []string{"", "b,a", "c,b,a", "b,c,a", "b,a,c"} {
		w := &pb.Warrior{Id: proto.String("owned"), Guid: proto.String("independent-guid"), Awake: &pb.Awake{Stage: proto.Int32(2)}, AwakenSkill: proto.String("c"), SkillOrder: proto.String(selected)}
		before := proto.Clone(w)
		base, insertion, err := s.P5BattleSkillTransport(w)
		if err != nil {
			t.Fatal(err)
		}
		ids, parts := strings.Split(base, ","), strings.Split(insertion, ",")
		if len(ids) != 3 || len(parts) != 2 {
			t.Fatal(base, insertion)
		}
		pos, _ := strconv.Atoi(parts[1])
		// Exact constructor insertion and ultimate swap from 018b..0285.
		actual := make([]string, 4)
		idx := pos
		if pos == 2 {
			idx = 3
		}
		actual[idx] = parts[0]
		for i, id := range ids {
			if actual[i] == "" {
				actual[i] = id
			} else {
				actual[i+1] = id
			}
		}
		if pos < 2 {
			actual[2], actual[3] = actual[3], actual[2]
		}
		want := []string{"a", "b", "u", "c"}
		if selected != "" {
			chosen := strings.Split(selected, ",")
			if len(chosen) == 2 {
				chosen = append(chosen, "c")
			}
			want = []string{chosen[0], chosen[1], "u", chosen[2]}
		}
		if !reflect.DeepEqual(actual, want) {
			t.Fatalf("%q actual=%v want=%v", selected, actual, want)
		}
		projected, err := s.P5WarriorProtocol(w)
		if err != nil || projected.GetAwakenSkill() != insertion || !proto.Equal(w, before) || projected.GetGuid() != w.GetGuid() {
			t.Fatal("source/protocol boundary", err)
		}
	}
	for _, invalid := range []string{"c,3", "c,-1", "x,2", "c,0,1"} {
		w := &pb.Warrior{Id: proto.String("owned"), Awake: &pb.Awake{Stage: proto.Int32(2)}, AwakenSkill: proto.String(invalid)}
		if _, _, err := s.P5BattleSkillTransport(w); err == nil {
			t.Fatal("invalid awakening", invalid)
		}
	}
}

// Every original partner retains its own original slots. Blue cards have no
// ultimate, and the six green cards have no skill definitions at all.
func TestP5AllOriginalPartnerSkillSlots(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range s.Tables["Hero"] {
		if row["heroClass"] != "0" {
			continue
		}
		w := &pb.Warrior{Id: proto.String(row["id"]), Awake: &pb.Awake{}}
		order, awake, err := s.P5BattleSkillTransport(w)
		if err != nil || awake != "" || order != strings.Join([]string{row["SkillId1"], row["SkillId2"], row["UltSkillId"]}, ",") {
			t.Fatalf("original partner %s lost original skill slots: %q, %q, %v", row["id"], order, awake, err)
		}
		if row["quality"] == "13" {
			w.SkillOrder = proto.String(row["SkillId2"] + "," + row["SkillId1"])
			order, awake, err = s.P5BattleSkillTransport(w)
			if err != nil || awake != "" || order != row["SkillId2"]+","+row["SkillId1"]+"," {
				t.Fatal("blue normal skill reorder", row["id"], order, err)
			}
		}
		if row["quality"] == "12" {
			w.SkillOrder = proto.String("043_1,043_2")
			if _, _, err := s.P5BattleSkillTransport(w); err == nil {
				t.Fatal("green card acquired forged normal skills", row["id"])
			}
			w.SkillOrder = nil
			w.AwakenSkill = proto.String("043_3,2")
			if _, _, err := s.P5BattleSkillTransport(w); err == nil {
				t.Fatal("green card acquired forged awakening", row["id"])
			}
		}
		count++
	}
	if count != 74 {
		t.Fatal("original partner scope changed", count)
	}
}
