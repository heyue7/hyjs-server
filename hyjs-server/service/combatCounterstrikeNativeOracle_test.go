package service

import (
	"encoding/base64"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原本机宿主实测的同一有限输入；只比反击 class7 数值，不授予新业务权威。
func TestP6CounterstrikeNativeOracleExpiredBuffScalar(t *testing.T) {
	encoded := "Cidjb3VudGVyc3RyaWtlLXNjYWxhci1vcmlnaW5hbC1vcmFjbGUtcjIQBxgCIucBCGUQARgBIh1maW5pdGUgb3JpZ2luYWwgY291bnRlcnN0cmlrZSq0AQoI" +
		"aGVyb18wNDcSEGNvdW50ZXItYXR0YWNrZXIaCHRhbF8zMDQwIgcwNDdfMSwsKmMJAAAAAACIw0ARAAAAAICELkEZAAAAAAAAAAAhAAAAAAAAAABJAAAAAAAA" +
		"JEBRAAAAAADAV0BZAAAAAAAAAABhAAAAAAAAFEBpAAAAAAAAAABxAAAAAACAQUB5AAAAAAAASUAwATgDQgkJAAAAAICELkFQAVkAAAAAAABZQGABagBwADIJ" +
		"CQAAAAAAAAAAKuMBCGYQARgBIh1maW5pdGUgb3JpZ2luYWwgY291bnRlcnN0cmlrZSqwAQoIaGVyb18wNDcSDWNvdW50ZXItb3duZXIaB3RhbF8zNTMiBzA0" +
		"N18xLCwqYwkAAAAAAEB/QBEAAAAAgIQuQRkAAAAAAAAAACEAAAAAAAAAAEkAAAAAAAAkQFEAAAAAAMBXQFkAAAAAAAAAAGEAAAAAAAAUQGkAAAAAAAAAAHEA" +
		"AAAAAAAAAHkAAAAAAABJQDABOANCCQkAAAAAgIQuQVABWQAAAAAAAFlAYAFqAHAAMgkJAAAAAAAAAABZAAAAAAAA8D9hAAAAAAAA8D9oDA=="
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || p6Digest(raw) != "8c9233bb5d10b8774467281d3ddd534c212db51699a75732c55be45b32fe46ca" {
		t.Fatal("original host oracle source changed", err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{
		Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(),
		MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(),
		AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range out.Events {
		if event.Kind == "counterstrike" && event.Round >= 7 {
			count++
			// 原 raw R7/R9/R11 共11条 class7 -65，Go trace记录正向伤害。
			if math.Float64bits(event.Value) != math.Float64bits(65) {
				t.Fatal("counterstrike differs from original host class7 export", event.Round, event.Time, event.Value)
			}
		}
	}
	if count != 11 || out.Authoritative {
		t.Fatal("original counterstrike count or diagnostic authority changed", count, out.Authoritative)
	}
}
