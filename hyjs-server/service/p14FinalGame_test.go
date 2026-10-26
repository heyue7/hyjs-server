package service

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14FinalGameActualPair(t *testing.T) *p14FinalGameProvenance {
	t.Helper()
	var root string
	for _, candidate := range []string{"../../analysis/p14/finals-draw-actual-native", "../../../analysis/p14/finals-draw-actual-native"} {
		if _, err := readTestAnalysis75(filepath.Join(candidate, "actual-ordered-pair-source.json")); err == nil {
			root = candidate
			break
		}
	}
	if root == "" {
		t.Fatal("actual155 ordered-pair proof missing")
	}
	var source struct {
		Pair p14FinalDrawPair `json:"pair"`
	}
	raw, err := readTestAnalysis75(filepath.Join(root, "actual-ordered-pair-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	left, err := readTestAnalysis75(filepath.Join(root, "actual-pair-left-BattlePlayer.pb"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := readTestAnalysis75(filepath.Join(root, "actual-pair-right-BattlePlayer.pb"))
	if err != nil {
		t.Fatal(err)
	}
	return &p14FinalGameProvenance{SeasonIndex: 61, RoundStage: constUtil.P14Final16Stage, GameIndex: 1, Pair: source.Pair, DrawHash: p6Digest([]byte("actual-draw-reference")), QualificationHash: p6Digest([]byte("actual-qualification-reference")), Policy: db.P14FinalGamePolicy{ID: 1, Enabled: true, Version: "test-prepublished-v1", Source: constUtil.P14FinalGameSource, PublishedAt: 1}, RulesHash: p6Digest([]byte(constUtil.P14FinalGameRules)), SealHashes: []string{p6Digest([]byte("origin1")), p6Digest([]byte("origin2"))}, Left: p14FinalSealMember{Kind: 3, PlayerID: source.Pair.LeftPlayerID, Battle: left}, Right: p14FinalSealMember{Kind: 3, PlayerID: source.Pair.RightPlayerID, Battle: right}}
}

// The real155 draw order is preserved; no player ID or initial battle property
// is rewritten to fit any previously certified single-match family.
func TestP14FinalGameActualOrderedPairInputHasNoOutcome(t *testing.T) {
	source := p14FinalGameActualPair(t)
	before, _ := json.Marshal(source)
	game, err := p14FinalGameBuild(source)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := p14FinalGameBuild(source)
	if err != nil {
		t.Fatal(err)
	}
	if game.ID != retry.ID || !bytes.Equal(game.Replay, retry.Replay) {
		t.Fatal("identical source changed input")
	}
	var replay pb.KFReplayData
	if err = proto.Unmarshal(game.Replay, &replay); err != nil {
		t.Fatal(err)
	}
	if len(replay.PlayerList) != 2 || replay.GetWarId() != game.ID || replay.GetRandSeed() <= 0 || replay.GetMaxRoundNum() != 40 || replay.WinPlayerId != nil || replay.Score != nil || replay.Time != nil || replay.RoomId != nil {
		t.Fatal("first game invented result/time/room or lost pair")
	}
	for i, expected := range [][]byte{source.Left.Battle, source.Right.Battle} {
		want := &pb.BattlePlayer{}
		if err = proto.Unmarshal(expected, want); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(want, replay.PlayerList[i]) {
			t.Fatalf("ordered actor%d mutated", i)
		}
	}
	after, _ := json.Marshal(source)
	if !bytes.Equal(before, after) {
		t.Fatal("producer input mutated")
	}
	source.SealHashes[0] = p6Digest([]byte("later-sealed-source"))
	changed, err := p14FinalGameBuild(source)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == game.ID || changed.ReplayHash == game.ReplayHash {
		t.Fatal("new source failed to bind game")
	}
}

func TestP14FinalGameRejectsMissingPairAndFutureSeries(t *testing.T) {
	for _, mutate := range []func(*p14FinalGameProvenance){
		func(p *p14FinalGameProvenance) { p.GameIndex = 2 },
		func(p *p14FinalGameProvenance) { p.RoundStage = 10 },
		func(p *p14FinalGameProvenance) { p.SealHashes = nil },
		func(p *p14FinalGameProvenance) { p.Left.PlayerID++ },
		func(p *p14FinalGameProvenance) { p.Right.Battle = nil },
		func(p *p14FinalGameProvenance) { p.Pair.LeftServerID++ },
		func(p *p14FinalGameProvenance) { p.RulesHash = "" },
	} {
		p := p14FinalGameActualPair(t)
		mutate(p)
		if _, err := p14FinalGameBuild(p); err == nil {
			t.Fatal("unsealed/future/mismatched pair accepted")
		}
	}
}

func TestP14FinalSealPublicationAndDestinationBinding(t *testing.T) {
	offer := &db.P14FinalFormationOffer{StageStart: 100}
	gate := &db.P14FinalGamePolicy{ID: 1, Enabled: true, Version: "v1", Source: constUtil.P14FinalGameSource, PublishedAt: 99}
	if !p14FinalGamePolicyValid(gate, offer) {
		t.Fatal("prepublished rule rejected")
	}
	gate.PublishedAt = 101
	if p14FinalGamePolicyValid(gate, offer) {
		t.Fatal("late rule accepted")
	}
	gate.PublishedAt = 99
	gate.Enabled = false
	if p14FinalGamePolicyValid(gate, offer) {
		t.Fatal("closed new rule accepted")
	}
	request := model.P14FinalSealImportRequest{ProtocolVersion: 1, SourceServerID: 2, DestinationServerID: 1, Snapshot: json.RawMessage(`{"seal":1}`)}
	first := p14FinalSealDeliveryID(request)
	request.DestinationServerID = 3
	if first == p14FinalSealDeliveryID(request) {
		t.Fatal("seal delivery reused across destinations")
	}
	request.DestinationServerID = 1
	request.Snapshot = json.RawMessage(`{"seal":2}`)
	if first == p14FinalSealDeliveryID(request) {
		t.Fatal("seal delivery ignored source content")
	}
}

// Header validation rejects missing source members and pre-lock seals before
// any source rows or remote delivery receipts are considered.
func TestP14FinalSealRequiresWholeRealOriginAfterLock(t *testing.T) {
	offer := p14FinalFormationPureOffer()
	draw := &p14FinalDrawSnapshot{SeasonIndex: 61, QualificationHash: offer.QualificationHash, Entrants: []p14FinalDrawEntrant{{ServerID: 2, PlayerID: 7}, {ServerID: 1, PlayerID: 7}, {ServerID: 2, PlayerID: 8}}}
	hash := p6Digest([]byte("common-real-draw"))
	body := p14FinalSealSnapshot{SeasonIndex: 61, RoundStage: 8, ServerID: 2, Policy: db.P14FinalGamePolicy{ID: 1, Enabled: true, Version: "v1", Source: constUtil.P14FinalGameSource, PublishedAt: offer.StageStart - 1}, RulesHash: p6Digest([]byte(constUtil.P14FinalGameRules)), DrawHash: hash, Offer: *offer, SealedAt: offer.LockAt, Members: []p14FinalSealMember{{PlayerID: 7}, {PlayerID: 8}}}
	now := time.UnixMilli(offer.FightStart)
	if err := p14FinalSealHeaderValidate(&body, draw, hash, now); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	for _, change := range []func(*p14FinalSealSnapshot){
		func(p *p14FinalSealSnapshot) { p.SealedAt = offer.LockAt - 1 },
		func(p *p14FinalSealSnapshot) { p.SealedAt = now.UnixMilli() + 1 },
		func(p *p14FinalSealSnapshot) { p.Members = p.Members[:1] },
		func(p *p14FinalSealSnapshot) { p.Members[1].PlayerID = 7 },
		func(p *p14FinalSealSnapshot) { p.ServerID = 3 },
		func(p *p14FinalSealSnapshot) { p.Policy.PublishedAt = offer.StageStart + 1 },
		func(p *p14FinalSealSnapshot) { p.DrawHash = "" },
		func(p *p14FinalSealSnapshot) { p.Offer.LockAt++ },
	} {
		var modified p14FinalSealSnapshot
		json.Unmarshal(raw, &modified)
		change(&modified)
		if p14FinalSealHeaderValidate(&modified, draw, hash, now) == nil {
			t.Fatal("incomplete or early/late source seal accepted")
		}
	}
}
