package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A new server fight must retain the same qualification and reject invalid
// actors; later supplied/persisted snapshots keep independent source checking.
func TestP6FreshAdmission64Boundary(t *testing.T) {
	s := p6OriginalTables(t)
	actor := p6Actor("hero_047", "00000000-0000-4000-8000-000000000001", "047_1,047_2,047_3", 3, 10000)
	source := *p6GenericInput(t, s, actor, 40)
	start := time.Now()
	previous, err := s.FreezeBattleInput(source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.ValidateP6GenericMechanisms(context.Background(), previous)
	if err != nil {
		t.Fatal(err)
	}
	oldTime := time.Since(start)
	start = time.Now()
	current, err := s.freezeAndQualifyP6GenericMechanisms(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	newTime := time.Since(start)
	got, err := s.ValidateP6GenericMechanisms(context.Background(), current)
	if err != nil || !reflect.DeepEqual(want, got) || !reflect.DeepEqual(previous, current) {
		t.Fatal("admission changed canonical input or qualification", err)
	}
	// A forged table plus a matching self-digest still fails the public verifier.
	current.Tables["Skill"][0]["SkillId"] = "forged-64"
	raw, err := json.Marshal(current.Tables)
	if err != nil {
		t.Fatal(err)
	}
	current.ConfigurationHash = p6Digest(raw)
	if _, err = s.ValidateP6GenericMechanisms(context.Background(), current); err == nil {
		t.Fatal("forged source admitted")
	}
	invalid := source
	invalid.Attacker = proto.Clone(source.Attacker).(*pb.BattlePlayer)
	invalid.Attacker.Formation[0].Id = proto.String("unknown-source-64")
	if _, err = s.freezeAndQualifyP6GenericMechanisms(context.Background(), invalid); err == nil {
		t.Fatal("unknown actor admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.freezeAndQualifyP6GenericMechanisms(ctx, source); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if _, err = s.freezeAndQualifyP6GenericMechanisms(nil, model.FrozenBattleInput{}); err == nil {
		t.Fatal("nil context admitted")
	}
	t.Logf("same original source first sample: old=%s new=%s", oldTime, newTime)
}
