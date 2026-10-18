package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminGamePlayerRenameKickPreservesOriginalReceipts keeps old paid name history,
// allows later original rename, revokes actual game sessions and never returns credentials.
func TestAdminGamePlayerRenameKickPreservesOriginalReceipts(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "player-operator", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(10000); return nil }); err != nil {
		t.Fatal(err)
	}
	renameCall(t, f, "原名甲", 730001)
	beforeP, beforeG := f.profile(t)
	oldState, err := readPlayerRenameState(beforeP, beforeG)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := AdminGamePlayer(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	out, err := AdminRenameGamePlayer(f.ctx, actor, f.id, model.AdminGamePlayerNameRequest{Version: detail.Version, Name: "运营乙", Reason: "修正昵称代表"})
	if err != nil {
		t.Fatal(err)
	}
	afterP, afterG := f.profile(t)
	state, err := readPlayerRenameState(afterP, afterG)
	if err != nil || state.Count != oldState.Count || state.Name != oldState.Name || state.AdminName != "运营乙" || string(state.Request) != string(oldState.Request) || string(state.Response) != string(oldState.Response) || afterP.GetDiamond() != beforeP.GetDiamond() {
		t.Fatal("admin rename changed original count, fee or frozen receipt", err)
	}
	if _, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String("原名甲")}, 730001); err == nil {
		t.Fatal("old game rename revived after admin override")
	}
	f.restart(t)
	next := renameCall(t, f, "正常丙", 730002)
	if len(next.GetCost()) != 1 {
		t.Fatal("later normal rename lost original paid count")
	}
	list, err := AdminGamePlayers(f.ctx, "正常丙", 1, 20)
	if err != nil || list.Total != 1 || len(list.Items) != 1 {
		t.Fatal("nickname pagination", err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	details, err := AdminGamePlayer(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	// Check actual JSON field names recursively; inventory guid is legitimate.
	var checkCredentialFields func(any)
	checkCredentialFields = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, nested := range value {
				switch strings.ToLower(key) {
				case "uid", "password", "passwordhash", "passwordsalt", "session", "sessionhex", "sessionkey":
					t.Fatal("credential field leaked in player DTO", key)
				}
				checkCredentialFields(nested)
			}
		case []any:
			for _, nested := range value {
				checkCredentialFields(nested)
			}
		}
	}
	checkCredentialFields(decoded)
	if _, err := AuthorizeAccount(f.ctx, f.id, row.Session); err != nil {
		t.Fatal(err)
	}
	kicked, err := AdminKickGamePlayer(f.ctx, actor, f.id, model.AdminGamePlayerKickRequest{Version: details.Version, Reason: "代表会话撤销"})
	if err != nil {
		t.Fatal(err)
	}
	if kicked.Version == details.Version {
		t.Fatal("kick did not change concurrency version")
	}
	if _, err := AuthorizeAccount(f.ctx, f.id, row.Session); err == nil {
		t.Fatal("old game session remained valid")
	}
	if _, err := AdminKickGamePlayer(f.ctx, actor, f.id, model.AdminGamePlayerKickRequest{Version: details.Version, Reason: "stale retry"}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale kick revoked a newer login", err)
	}
	_ = out
}
