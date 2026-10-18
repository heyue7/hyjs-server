package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p13TechPublish(t *testing.T, at time.Time) {
	t.Helper()
	zero := int64(0)
	directory := p13GuildTechDirectory{InitialLevel: &zero, HeroStatue: []string{"peratt", "cri", "hit"}, GuardTree: []string{"perhp", "perpdef", "permdef", "anticri", "dodge"}}
	raw, err := json.Marshal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P13GuildTechPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "local-test-directory-initial0-v1", "directory_json": string(raw), "directory_hash": p6Digest(raw), "published_at": at.UnixMilli(), "source": constUtil.P13GuildTechSource}).Error; err != nil {
		t.Fatal(err)
	}
}

// p13TechSetup supplies a finite existing wallet/items only. All guild funds
// below are earned by actual original donations over three controlled days.
func p13TechSetup(t *testing.T) (*p1Fixture, []int64, time.Time, string) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP13GuildTech(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P13GuildTechUpgrade{}, &db.P13GuildTechOffer{}, &db.P13GuildTechPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP13GuildTech(f.ctx); err != nil {
		t.Fatal(err)
	}
	p13GuildOpen(t, true)
	first := time.Now().Add(-48*time.Hour - time.Minute)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("原科技实升")}, 1, first)
	guild := p13GuildID(t, f, ids[0], first)
	for index, id := range ids[1:] {
		p13Join(t, f, ids[0], id, guild, int64(10+index), first)
	}
	for day := 0; day < 3; day++ {
		count := 15
		if day == 2 {
			count = 10
		}
		for _, id := range ids {
			for index := 0; index < count; index++ {
				p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, int64(1000+day*100+index), first.Add(time.Duration(day)*24*time.Hour))
			}
		}
	}
	now := first.Add(48*time.Hour + time.Second)
	for index := int64(0); index < 30; index++ {
		p13GuildCall(t, f, ids[0], &pb.CSUnionDonate{DonateType: proto.String("badge")}, 2000+index, now)
	}
	if _, err := f.s.p13GuildOpenBuilder(f.ctx, ids[0], &pb.CSUnionOpenBuilder{Type: proto.String("HeroStatue")}, 3000, now); err != nil {
		t.Fatal(err)
	}
	var assets db.P13Guild
	if err := db.GetIns().First(&assets, "id=?", guild).Error; err != nil || assets.Coin != 10000000 || assets.Badge != 10 || assets.HeroStatueOpen != 1 {
		t.Fatal("actual 20m/30 donations -> original building10m/20", assets, err)
	}
	return f, ids, now, guild
}
func p13TechUpgrade(t *testing.T, f *p1Fixture, id int64, kind, building string, stamp int64, at time.Time) {
	t.Helper()
	var req proto.Message = &pb.CSUnionHeroStatueUpgrade{Type: proto.String(kind)}
	if building == constUtil.P13GuildTechGuardTree {
		req = &pb.CSUnionGuardTreeUpgrade{Type: proto.String(kind)}
	}
	messages, err := f.s.p13GuildTechUpgrade(f.ctx, id, req, kind, building, stamp, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 {
		t.Fatal("missing original empty response")
	}
	raw, err := proto.Marshal(messages[0])
	if err != nil || len(raw) != 0 {
		t.Fatal("upgrade response must retain original empty SC", err)
	}
}
func TestP13GuildTechRealFundsOriginalUpgradeOnceFrozenColdAndMembership(t *testing.T) {
	f, ids, now, guild := p13TechSetup(t)
	req := &pb.CSUnionHeroStatueUpgrade{Type: proto.String("peratt")}
	if _, err := f.s.p13GuildTechUpgrade(f.ctx, ids[0], req, "peratt", constUtil.P13GuildTechHeroStatue, 4000, now); err == nil {
		t.Fatal("disabled technology allowed")
	}
	p13TechPublish(t, now.Add(-time.Second))
	if _, err := f.s.p13GuildTechUpgrade(f.ctx, ids[1], req, "peratt", constUtil.P13GuildTechHeroStatue, 4000, now); err == nil {
		t.Fatal("ordinary member spent guild funds")
	}
	info, err := f.s.p13GuildTechQuery(f.ctx, ids[0], constUtil.P13GuildTechHeroStatue, now)
	if err != nil {
		t.Fatal(err)
	}
	initial := info[0].(*pb.SCUnionHeroStatueInfo)
	if len(initial.GetUnionPropAddAtt()) != 3 || initial.GetUnionPropAddAtt()[0].GetLevel() != 0 || initial.GetUnionPropAddAtt()[0].GetProp() != "peratt,0" {
		t.Fatal("explicit local initial0 directory", initial)
	}
	inactive, hash, err := f.s.P13GuildTechProperties(f.ctx, db.GetIns(), ids[1], now)
	if err != nil || hash != "" || !proto.Equal(inactive, &pb.BaseProp{}) {
		t.Fatal("query granted a free level", inactive, err)
	}
	p13TechUpgrade(t, f, ids[0], "peratt", constUtil.P13GuildTechHeroStatue, 4000, now)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.p13GuildTechUpgrade(f.ctx, ids[0], req, "peratt", constUtil.P13GuildTechHeroStatue, 4001, now.Add(time.Second))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var assets db.P13Guild
	if err = db.GetIns().First(&assets, "id=?", guild).Error; err != nil || assets.Coin != 0 || assets.Badge != 0 {
		t.Fatal("original level2 guild debit not once", assets, err)
	}
	var count int64
	if err = db.GetIns().Model(&db.P13GuildTechUpgrade{}).Where("guild_id=?", guild).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("actual level transitions", count, err)
	}
	prop, source, err := f.s.P13GuildTechProperties(f.ctx, db.GetIns(), ids[1], now.Add(2*time.Second))
	if err != nil || prop.GetPeratt() != 2 || source == "" {
		t.Fatal("ordinary member did not receive actual guild asset", prop, err)
	}
	if err = db.GetIns().Model(&db.P13GuildTechPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "version": "closed-next-version", "directory_json": "{}", "directory_hash": "changed"}).Error; err != nil {
		t.Fatal(err)
	}
	p13TechUpgrade(t, f, ids[0], "peratt", constUtil.P13GuildTechHeroStatue, 4001, now.Add(2*time.Second))
	sqlDB, _ := db.GetIns().DB()
	sqlDB.Close()
	f.open(t)
	actual, hash, err := f.s.P13GuildTechProperties(f.ctx, db.GetIns(), ids[1], now.Add(time.Minute))
	if err != nil || hash != source || !proto.Equal(actual, prop) {
		t.Fatal("policy close/cold changed frozen upgrades", actual, err)
	}
	p13GuildCall(t, f, ids[0], &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(ids[1])}, 5000, now.Add(time.Minute))
	actual, hash, err = f.s.P13GuildTechProperties(f.ctx, db.GetIns(), ids[1], now.Add(time.Minute))
	if err != nil || hash != "" || actual == nil || !proto.Equal(actual, &pb.BaseProp{}) {
		t.Fatal("kicked member retained guild asset", actual, err)
	}
	if err = db.GetIns().Model(&db.P13GuildTechUpgrade{}).Where("guild_id=?", guild).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("member revocation erased real upgrades", count, err)
	}
}
func TestP13GuildTechAllOriginalEightTypesHighestTenAndCostConservation(t *testing.T) {
	f, ids, now, guild := p13TechSetup(t)
	p13TechPublish(t, now.Add(-time.Second))
	// Explicit accumulated-fund boundary fixture for all original highest tiers;
	// this test does not claim these extra funds came from the native source chain.
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": int64(2300000000), "badge": int32(2300)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildOpenBuilder(f.ctx, ids[0], &pb.CSUnionOpenBuilder{Type: proto.String("GuardTree")}, 6000, now); err != nil {
		t.Fatal(err)
	}
	original, _, err := f.s.p13GuildTechOriginalRows()
	if err != nil {
		t.Fatal(err)
	}
	spentCoin, spentBadge := int64(10000000), int32(20)
	stamp := int64(6100)
	directoryRaw := &db.P13GuildTechPolicy{}
	if err = db.GetIns().First(directoryRaw, 1).Error; err != nil {
		t.Fatal(err)
	}
	directory, err := p13GuildTechDirectoryRead(directoryRaw.DirectoryJSON, directoryRaw.DirectoryHash)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"peratt", "perhp", "perpdef", "permdef", "cri", "hit", "anticri", "dodge"} {
		for level := int64(1); level <= 10; level++ {
			row, err := p13GuildTechRowAt(original, kind, level)
			if err != nil {
				t.Fatal(err)
			}
			stamp++
			p13TechUpgrade(t, f, ids[0], kind, p13GuildTechBuilding(directory, kind), stamp, now.Add(time.Duration(stamp-6100)*time.Millisecond))
			spentCoin += row.Coin
			spentBadge += row.Badge
		}
		var req proto.Message = &pb.CSUnionHeroStatueUpgrade{Type: proto.String(kind)}
		if p13GuildTechBuilding(directory, kind) == constUtil.P13GuildTechGuardTree {
			req = &pb.CSUnionGuardTreeUpgrade{Type: proto.String(kind)}
		}
		if _, err := f.s.p13GuildTechUpgrade(f.ctx, ids[0], req, kind, p13GuildTechBuilding(directory, kind), stamp+1000, now.Add(time.Second)); err == nil {
			t.Fatal("original maximum10 exceeded", kind)
		}
	}
	var assets db.P13Guild
	if err = db.GetIns().First(&assets, "id=?", guild).Error; err != nil || assets.Coin != 2300000000-spentCoin || assets.Badge != 2300-spentBadge {
		t.Fatal("all original80 costs not conserved", assets.Coin, spentCoin, assets.Badge, spentBadge, err)
	}
	prop, source, err := f.s.P13GuildTechProperties(f.ctx, db.GetIns(), ids[2], now.Add(time.Second))
	if err != nil || source == "" || prop.GetPeratt() != 10 || prop.GetPerhp() != 10 || prop.GetPerpdef() != 10 || prop.GetPermdef() != 10 || prop.GetCri() != 10 || prop.GetHit() != 10 || prop.GetAntiCri() != 10 || prop.GetDodge() != 10 {
		t.Fatal("all original highest-tier properties", prop, err)
	}
}
