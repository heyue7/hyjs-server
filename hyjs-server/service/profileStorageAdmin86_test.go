package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func adminStorage86Warrior(t *testing.T, guid string, extra json.RawMessage) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(&pb.Warrior{Id: proto.String("hero_001"), Guid: proto.String(guid), Level: proto.Int32(1), Breached: &pb.Breached{Level: proto.Int32(1)}})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	fields["futureExtra"] = extra
	encoded, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func adminStorage86Read(t *testing.T, id int64) (db.Profile, map[string]json.RawMessage, []map[string]json.RawMessage) {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil || row.Game == nil {
		t.Fatalf("read saved profile: %v", err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*row.Game), &stored); err != nil {
		t.Fatal(err)
	}
	expanded, err := model.ExpandProfileGame86([]byte(*row.Game))
	if err != nil {
		t.Fatalf("expand saved profile: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(expanded, &fields); err != nil {
		t.Fatal(err)
	}
	var warriors []map[string]json.RawMessage
	if err := json.Unmarshal(fields["warriors"], &warriors); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(fields["futureTop"], []byte(`"keep"`)) || !bytes.Contains(fields["futureTop"], []byte("9223372036854775807")) {
		t.Fatal("unknown top-level field changed")
	}
	return row, stored, warriors
}

func adminStorage86Find(t *testing.T, warriors []map[string]json.RawMessage, guid string) map[string]json.RawMessage {
	t.Helper()
	for _, warrior := range warriors {
		var id string
		if err := json.Unmarshal(warrior["guid"], &id); err != nil {
			t.Fatal(err)
		}
		if id == guid {
			return warrior
		}
	}
	return nil
}

func TestAdminProfileStorage86RosterCRUDAndCold(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(f.ctx); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "storage86-editor", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, _ *model.GameState) error {
		player.Level = proto.Int32(20)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil || row.Game == nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*row.Game), &fields); err != nil {
		t.Fatal(err)
	}
	var warriors []json.RawMessage
	if err := json.Unmarshal(fields["warriors"], &warriors); err != nil {
		t.Fatal(err)
	}
	largeExtra, err := json.Marshal(map[string]string{"token": "keep-a", "padding": strings.Repeat("x", constUtil.ProfileWarriorsCompressMin86)})
	if err != nil {
		t.Fatal(err)
	}
	warriors = append(warriors, adminStorage86Warrior(t, "storage86-a", largeExtra), adminStorage86Warrior(t, "storage86-b", json.RawMessage(`{"token":"keep-b","counter":9223372036854775807}`)))
	fields["warriors"], err = json.Marshal(warriors)
	if err != nil {
		t.Fatal(err)
	}
	fields["futureTop"] = json.RawMessage(`{"keep":true,"counter":9223372036854775807}`)
	legacy, err := json.Marshal(fields)
	if err != nil || len(fields["warriors"]) <= constUtil.ProfileWarriorsCompressMin86 {
		t.Fatalf("legacy large roster fixture: %v", err)
	}
	if err := db.GetIns().Model(&row).Update("game", string(legacy)).Error; err != nil {
		t.Fatal(err)
	}
	readVersion := func() string {
		t.Helper()
		inventory, err := f.s.AdminPlayerInventory(ctx, f.id, "warrior", "", nil, 1, 100)
		if err != nil {
			t.Fatal(err)
		}
		return inventory.Version
	}
	mutate := func(operation, guid string, level int64) {
		t.Helper()
		req := model.AdminPlayerInventoryRequest{Version: readVersion(), RequestID: uuid.NewString(), Operation: operation, Category: "warrior", Key: guid, Reason: "isolated roster storage regression"}
		if operation == "update" {
			req.Attributes = map[string]int64{"level": level}
		}
		if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, req); err != nil {
			t.Fatalf("%s %s: %v", operation, guid, err)
		}
	}
	check := func(warriors []map[string]json.RawMessage, guid string, level int) {
		t.Helper()
		warrior := adminStorage86Find(t, warriors, guid)
		if warrior == nil {
			t.Fatalf("missing warrior %s", guid)
		}
		var got int
		if err := json.Unmarshal(warrior["level"], &got); err != nil || got != level {
			t.Fatalf("warrior %s level %d, error %v", guid, got, err)
		}
		if !bytes.Contains(warrior["futureExtra"], []byte("keep-"+strings.TrimPrefix(guid, "storage86-"))) {
			t.Fatalf("unknown field lost for %s", guid)
		}
	}
	// A legacy large array must survive a real editor update and become compact.
	mutate("update", "storage86-a", 2)
	row, stored, warriorsAfter := adminStorage86Read(t, f.id)
	if !bytes.HasPrefix(bytes.TrimSpace(stored["warriors"]), []byte("{")) {
		t.Fatal("large legacy roster was not compacted after edit")
	}
	check(warriorsAfter, "storage86-a", 2)
	check(warriorsAfter, "storage86-b", 1)
	if !bytes.Contains(adminStorage86Find(t, warriorsAfter, "storage86-b")["futureExtra"], []byte("9223372036854775807")) {
		t.Fatal("unknown integer lost precision")
	}
	var unchanged model.GameState
	if err := json.Unmarshal([]byte(*row.Game), &unchanged); err != nil {
		t.Fatal(err)
	}
	noChange, err := adminMergeProfileGame86(*row.Game, &unchanged, &unchanged)
	if err != nil || noChange != *row.Game {
		t.Fatalf("no-op did not retain exact stored bytes: %v", err)
	}
	// Edit a second GUID from an already compacted roster, then remove the large
	// unknown payload and verify the roster crosses back below the threshold.
	mutate("update", "storage86-b", 3)
	_, stored, warriorsAfter = adminStorage86Read(t, f.id)
	if !bytes.HasPrefix(bytes.TrimSpace(stored["warriors"]), []byte("{")) {
		t.Fatal("edited compact roster unexpectedly expanded")
	}
	check(warriorsAfter, "storage86-a", 2)
	check(warriorsAfter, "storage86-b", 3)
	mutate("remove", "storage86-a", 0)
	row, stored, warriorsAfter = adminStorage86Read(t, f.id)
	if !bytes.HasPrefix(bytes.TrimSpace(stored["warriors"]), []byte("[")) || adminStorage86Find(t, warriorsAfter, "storage86-a") != nil {
		t.Fatal("removing the large entry did not restore the small array")
	}
	check(warriorsAfter, "storage86-b", 3)
	if !bytes.Contains(adminStorage86Find(t, warriorsAfter, "storage86-b")["futureExtra"], []byte("9223372036854775807")) {
		t.Fatal("unknown integer was lost after removal")
	}
	f.restart(t)
	_, coldStored, coldWarriors := adminStorage86Read(t, f.id)
	if !bytes.Equal(stored["warriors"], coldStored["warriors"]) {
		t.Fatal("roster changed during cold reload")
	}
	check(coldWarriors, "storage86-b", 3)
	// Make a syntactically valid but invalid codec payload. The actual admin
	// transaction must reject it without changing either saved column or audit.
	var savedFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*row.Game), &savedFields); err != nil {
		t.Fatal(err)
	}
	savedFields["warriors"] = json.RawMessage(`{"codec":"hyjs-warriors-zlib-v1","bytes":1,"sha256":"0000000000000000000000000000000000000000000000000000000000000000","data":"eA=="}`)
	corrupt, err := json.Marshal(savedFields)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("game", string(corrupt)).Error; err != nil {
		t.Fatal(err)
	}
	var before db.Profile
	if err := db.GetIns().First(&before, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := model.ExpandProfileGame86([]byte(*before.Game)); err == nil {
		t.Fatal("damaged roster passed raw expansion")
	}
	if _, _, err := decodeProfile(&before); err == nil {
		t.Fatal("damaged roster passed full profile decoding")
	}
	if _, err := adminMergeProfileGame86(*before.Game, &unchanged, &unchanged); err == nil {
		t.Fatal("damaged roster passed the admin merge")
	}
	var auditCount int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	// Derive the current version from the corrupted row, so a version conflict
	// cannot make the rejection appear successful.
	req := model.AdminPlayerInventoryRequest{Version: adminInventoryVersion(before, nil), RequestID: uuid.NewString(), Operation: "update", Category: "warrior", Key: "storage86-b", Attributes: map[string]int64{"level": 4}, Reason: "reject damaged roster"}
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, req); err == nil {
		t.Fatal("corrupted roster was accepted")
	}
	var after db.Profile
	var afterAudit int64
	if err := db.GetIns().First(&after, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminAudit{}).Count(&afterAudit).Error; err != nil {
		t.Fatal(err)
	}
	if *after.Player != *before.Player || *after.Game != *before.Game || afterAudit != auditCount {
		t.Fatal("damaged roster changed stored profile or audit")
	}
}
