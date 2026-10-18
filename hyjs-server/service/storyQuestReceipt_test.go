package service

import (
	"bytes"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func storyQuestReceiptSetup(t *testing.T, questID string) *p1Fixture {
	t.Helper()
	f := p1Setup(t)
	if err := f.s.InitializeStory(f.ctx); err != nil {
		t.Fatal(err)
	}
	target := f.s.table("StoryQuest", "questId", questID)
	mission := f.s.table("CopyMission", "MissionId", target["missionId"])
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, game *model.GameState) error {
		// Finite isolated prerequisite state only; this is not a natural replay of earlier battles/dialogue.
		game.Quests = nil
		for _, row := range f.s.Tables["StoryQuest"] {
			if tableNumber(row, "order") < tableNumber(target, "order") {
				game.Quests = append(game.Quests, row["questId"])
			}
		}
		game.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String(mission["ChapterId"]), MissionId: proto.String(mission["MissionId"]), Star: proto.Int32(3)}}
		f.s.refreshCopyProgress(game)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestStoryQuestReceiptNPCConcurrentExactColdExpiryAndOriginalChapterGate(t *testing.T) {
	f := storyQuestReceiptSetup(t, "quest_16")
	now := time.Now()
	before, game := f.profile(t)
	mission12 := f.s.table("CopyMission", "MissionId", "12")
	if f.s.copyMissionOpen(game, mission12) {
		t.Fatal("chapter three opened before the original NPC quest")
	}
	var (
		wg      sync.WaitGroup
		results = make(chan []proto.Message, 8)
		errors  = make(chan error, 8)
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, err := f.s.storyQuestRewardAt(f.ctx, f.id, "quest_16", 100, now)
			results <- messages
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first proto.Message
	for messages := range results {
		if first == nil {
			first = messages[0]
		}
		if !proto.Equal(messages[0], first) {
			t.Fatal("same request failed to recover the exact original reward frame")
		}
	}
	after, game := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 50000 || after.GetDiamond()-before.GetDiamond() != 30 || !slices.Contains(game.Quests, "quest_16") || !f.s.copyMissionOpen(game, mission12) {
		t.Fatal("original NPC quest did not grant once and open mission12")
	}
	var receipt db.StoryQuestReceipt
	if err := db.GetIns().First(&receipt, "player_id=? AND stamp=?", f.id, 100).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(first)
	if err != nil || !bytes.Equal(raw, receipt.Response) {
		t.Fatal("committed response bytes differ", err)
	}
	if _, err = f.s.storyQuestRewardAt(f.ctx, f.id, "quest_17", 100, now); err == nil {
		t.Fatal("same stamp changed its quest")
	}
	if _, err = f.s.storyQuestRewardAt(f.ctx, f.id, "quest_16", 101, now); err == nil {
		t.Fatal("new stamp duplicated a claimed quest")
	}
	if _, err = f.s.storyQuestReward(f.ctx, f.id, "quest_16"); err == nil {
		t.Fatal("legacy zero-stamp duplicate semantics changed")
	}
	f.restart(t)
	restored, err := f.s.storyQuestRewardAt(f.ctx, f.id, "quest_16", 100, now.Add(time.Second))
	if err != nil || !proto.Equal(restored[0], first) {
		t.Fatal("cold exact retry failed", err)
	}
	// Corrupted frame bytes must fail provenance before any wallet/progress mutation.
	if err := db.GetIns().Model(&db.StoryQuestReceipt{}).Where("player_id=? AND stamp=?", f.id, 100).Update("response", []byte{0}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.storyQuestRewardAt(f.ctx, f.id, "quest_16", 100, now.Add(time.Second)); err == nil {
		t.Fatal("corrupted durable reward frame passed provenance")
	}
	if err := db.GetIns().Model(&db.StoryQuestReceipt{}).Where("player_id=? AND stamp=?", f.id, 100).Update("response", receipt.Response).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.storyQuestRewardAt(f.ctx, f.id, "quest_16", 100, now.Add(24*time.Hour)); err == nil || err.Error() != constUtil.StoryQuestReceiptExpired {
		t.Fatal("expired reward frame was replayed", err)
	}
	final, game := f.profile(t)
	if final.GetCoin() != after.GetCoin() || final.GetDiamond() != after.GetDiamond() || !slices.Contains(game.Quests, "quest_16") || !f.s.copyMissionOpen(game, mission12) {
		t.Fatal("recovery or expiry changed already committed progress/reward")
	}
}

func TestStoryQuestReceiptFullBagRollbackThenActualEquipmentGrowthRetiresOldFrame(t *testing.T) {
	f := storyQuestReceiptSetup(t, "quest_18")
	if err := f.s.InitEquipmentMutations(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, game *model.GameState) error {
		player.Level = proto.Int32(30)
		player.EquipBagSize = proto.Int32(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err := f.s.storyQuestReward(f.ctx, f.id, "quest_18", 200); err == nil {
		t.Fatal("full equipment bag accepted an original equipment reward")
	}
	var count int64
	if err := db.GetIns().Model(&db.StoryQuestReceipt{}).Where("player_id=?", f.id).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed reward left a replay receipt", err)
	}
	afterFail, game := f.profile(t)
	if afterFail.GetCoin() != before.GetCoin() || afterFail.GetDiamond() != before.GetDiamond() || slices.Contains(game.Quests, "quest_18") || len(game.Equipment) != 0 {
		t.Fatal("full bag left partial currency, equipment or progress")
	}
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, game *model.GameState) error {
		player.EquipBagSize = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.storyQuestReward(f.ctx, f.id, "quest_18", 200)
	if err != nil {
		t.Fatal(err)
	}
	award := messages[0].(*pb.SCQuestReward)
	if len(award.GetGet().GetEquips()) != 1 {
		t.Fatal("original reward did not create its real equipment instance")
	}
	if replay, err := f.s.storyQuestReward(f.ctx, f.id, "quest_18", 200); err != nil || !proto.Equal(replay[0], award) {
		t.Fatal("unchanged owned instance frame was not recoverable", err)
	}
	equipment := award.GetGet().GetEquips()[0]
	quality := f.s.table("Equip", "id", equipment.GetEquipId())["quality"]
	var cost int64
	for _, row := range f.s.Tables["EquipExp"] {
		if row["quality"] == quality && row["level"] == "1" {
			cost, err = strconv.ParseInt(row["coin"], 10, 32)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if cost <= 0 {
		t.Fatal("original equipment growth coin quote missing")
	}
	request := &pb.CSEquipStrengthen{Guid: equipment.Guid, Times: proto.Int32(1), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(cost))}}}
	if _, err = f.s.equipmentStrengthenStamped(f.ctx, f.id, request, 201); err != nil {
		t.Fatal("actual awarded equipment growth failed", err)
	}
	current, _ := f.profile(t)
	if _, err = f.s.storyQuestReward(f.ctx, f.id, "quest_18", 200); err == nil {
		t.Fatal("old equipment body was replayed after actual growth")
	}
	f.restart(t)
	if _, err = f.s.storyQuestReward(f.ctx, f.id, "quest_18", 200); err == nil {
		t.Fatal("old equipment body survived cold recovery after growth")
	}
	final, game := f.profile(t)
	if final.GetCoin() != current.GetCoin() || !slices.Contains(game.Quests, "quest_18") || game.Equipment[0].GetLevel() <= equipment.GetLevel() {
		t.Fatal("refused stale reward overwrote current equipment/progress/wallet")
	}
}
