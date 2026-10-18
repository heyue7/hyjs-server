package service

import (
	"hyjs-server/db"
	"testing"
)

// TestP13UnionFightOriginalCache 验证重复解析复用、每次blob损坏仍拒绝、版本/ServerID隔离及容量上限。
func TestP13UnionFightOriginalCacheActualFrozenSHAReuseIntegrityAndBoundedServerIdentity(t *testing.T) {
	f, _, _, _, match := p13UFFieldSetup(t)
	first, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil || first != second {
		t.Fatal("verified same SHA still reparsed original all tables", err)
	}
	var entry db.P13UnionFightEntry
	if err = db.GetIns().First(&entry, "guild_id=? AND day=?", match.LeftGuildID, p13UFDay(p13UFMondays())).Error; err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), entry.FrozenTables...)
	if err = db.GetIns().Model(&entry).Update("frozen_tables", []byte("{} ")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldOriginal(match, db.GetIns()); err == nil {
		t.Fatal("cache masked current blob SHA corruption")
	}
	if err = db.GetIns().Model(&entry).Update("frozen_tables", original).Error; err != nil {
		t.Fatal(err)
	}
	for _, server := range []int64{1, 2, 3} {
		origin := &GameService{ServerID: server}
		parsed, err := origin.p13UFFieldOriginal(match, db.GetIns())
		if err != nil || parsed.ServerID != server {
			t.Fatal("immutable cache crossed original deployment identity", err)
		}
	}
	p13UFOriginalCache.Lock()
	defer p13UFOriginalCache.Unlock()
	if len(p13UFOriginalCache.Entries) > 2 || len(p13UFOriginalCache.Order) > 2 {
		t.Fatal("unbounded full original-table allocation")
	}
}
