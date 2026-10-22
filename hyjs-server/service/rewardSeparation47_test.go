package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Synthetic immutable receipts isolate source-version compatibility from native combat.
func TestP11ArenaRankRewardVersionedReceipts47(t *testing.T) {
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.P6BattleRecord{}, &db.P11ArenaPrizeAward{}); err != nil {
		t.Fatal(err)
	}
	input := p6Input(t, f.s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 12)
	if input.AppliedProperties == nil {
		input.AppliedProperties = map[string][]string{}
	}
	input.AppliedProperties["arenaRankReward"] = []string{constUtil.P11ArenaRankRewardSource, "100"}
	rawInput, _ := json.Marshal(input)
	record := db.P6BattleRecord{ID: "reward47-input", PlayerID: input.Attacker.GetPlayerId(), Input: rawInput, InputHash: p6Digest(rawInput), Seed: input.Seed, ConfigurationHash: input.ConfigurationHash, EngineVersion: input.EngineVersion, ClientVersion: input.ClientVersion}
	if err := db.GetIns().Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	month, day := p11ArenaPrizeMonth(now)
	for i, source := range []string{constUtil.P11ArenaRankRewardSource, constUtil.P11ArenaRankRewardCenterSource, constUtil.P11ArenaRankRewardSource, constUtil.P11ArenaRankRewardCenterSource} {
		row := db.P11ArenaFightSettlement{FightID: fmt.Sprint("reward47-", i), PlayerID: f.id, WinnerID: f.id + 1, P6RecordID: record.ID, AttackerOldRank: 100, AttackerNewRank: 100, CreatedAt: now.UnixMilli()}
		response := &pb.SCPvpFightResult{FightId: proto.String(row.FightID), Result: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("energy"), Num: proto.Int32(-2)}}}}
		base, _ := proto.MarshalOptions{Deterministic: true}.Marshal(p11ArenaPrizeBase(response))
		encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		row.ResponseHash = p6Digest(encoded)
		award := db.P11ArenaPrizeAward{FightID: row.FightID, PlayerID: f.id, WinnerID: row.WinnerID, P6RecordID: record.ID, CreatedAt: row.CreatedAt, Month: month, Day: day, Status: constUtil.P11ArenaPrizeLost, DrawJSON: "[]", BoxesJSON: "null", RankSource: source, RankBestBefore: 100, RankBestAfter: 100, BaseResponseHash: p6Digest(base), FinalResponseHash: row.ResponseHash}
		if i >= 2 {
			row.WinnerID, row.AttackerNewRank = f.id, 91
			award.WinnerID, award.RankBestAfter, award.RankDiamond = f.id, 91, 9
			draw := &p11ArenaRegularCoinDraw{Source: constUtil.P11ArenaPrizeRegularCoinSource, Reason: constUtil.P11ArenaPrizeDisabled, Values: []int64{0, 0, 0}}
			cards, _, err := p11ArenaRegularCoinCards(draw.Reason, draw)
			if err != nil {
				t.Fatal(err)
			}
			response.BoxList = cards
			gain := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: cards[0].Num}}}
			p1AppendGain(response.Result, gain)
			award.Status = constUtil.P11ArenaPrizeRegularCoin
			drawRaw, _ := json.Marshal(draw)
			award.DrawJSON = string(drawRaw)
			boxesRaw, _ := json.Marshal(cards)
			award.BoxesJSON = string(boxesRaw)
			award.GrantPB, _ = proto.MarshalOptions{Deterministic: true}.Marshal(gain)
			if source == constUtil.P11ArenaRankRewardSource {
				p1AppendGain(response.Result, &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("diamond"), Num: proto.Int32(9)}}})
			} else {
				if _, err = f.s.EnqueueReward(db.GetIns(), f.id, p11ArenaRankRewardCenterKey(row.FightID), constUtil.P11ArenaRankRewardCenterTitle, "历史最高排名提升至第91名。", "diamond,9", 0); err != nil {
					t.Fatal(err)
				}
			}
			encoded, _ = proto.MarshalOptions{Deterministic: true}.Marshal(response)
			row.ResponseHash = p6Digest(encoded)
			award.FinalResponseHash = row.ResponseHash
		}
		award.SourceHash = p11ArenaPrizeAwardHash(award)
		if err := db.GetIns().Create(&award).Error; err != nil {
			t.Fatal(err)
		}
		if err := ValidateP11ArenaFightPrizeReceipt(db.GetIns(), &row, response); err != nil {
			t.Fatal(source, err)
		}
		if i < 3 && p1Count(t, &db.RewardDelivery{}, "player_id=?", f.id) != 0 {
			t.Fatal("loss created delivery")
		}
		// Same immutable facts with an unknown version must never widen the receipt.
		award.RankSource = "untrusted-version"
		award.SourceHash = p11ArenaPrizeAwardHash(award)
		if err := db.GetIns().Save(&award).Error; err != nil {
			t.Fatal(err)
		}
		if ValidateP11ArenaFightPrizeReceipt(db.GetIns(), &row, response) == nil {
			t.Fatal("unknown version accepted")
		}
	}
}

func TestP11ArenaRankRewardAttachmentIdentity47(t *testing.T) {
	f := p1Setup(t)
	row := &db.P11ArenaFightSettlement{FightID: "attachment47", PlayerID: f.id}
	award := &db.P11ArenaPrizeAward{RankDiamond: 9, RankBestAfter: 91}
	reward, err := f.s.EnqueueRewardNotice(db.GetIns(), f.id, p11ArenaRankRewardCenterKey(row.FightID), constUtil.P11ArenaRankRewardCenterTitle, "历史最高排名提升至第91名。", "diamond,9", 0)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if err = validateP11ArenaRankRewardCenterReceipt(db.GetIns(), row, award); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.MailMessage{}, "player_id=?", f.id) != 0 {
		t.Fatal("implicit mail")
	}
	for i := 0; i < 2; i++ {
		if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	f.restart(t)
	if err = validateP11ArenaRankRewardCenterReceipt(db.GetIns(), row, award); err != nil {
		t.Fatal("claimed cold receipt", err)
	}
	after, _ := f.profile(t)
	if after.GetDiamond() != before.GetDiamond()+9 {
		t.Fatal("claim repeated")
	}
	if err = db.GetIns().First(reward, reward.ID).Error; err != nil {
		t.Fatal(err)
	}
	for column, value := range map[string]any{"player_id": f.id + 1, "source_key": "foreign-source", "rewards": "diamond,10", "title": "wrong", "content": "wrong", "expires_at": int64(1)} {
		if err = db.GetIns().Model(&db.RewardDelivery{}).Where("id=?", reward.ID).Update(column, value).Error; err != nil {
			t.Fatal(err)
		}
		if validateP11ArenaRankRewardCenterReceipt(db.GetIns(), row, award) == nil {
			t.Fatal("altered attachment accepted", column)
		}
		if err = db.GetIns().Save(reward).Error; err != nil {
			t.Fatal(err)
		}
	}
}
