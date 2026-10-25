package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// These inputs must fail before the global database or password worker is used.
func TestContentModerationNameEntrypointsRejectBeforeDatabase(t *testing.T) {
	ctx := context.Background()
	game := &GameService{P1Ready: true}
	cases := []struct {
		name string
		call func() error
	}{
		{"credential username", func() error { _, e := gameAccountNewProfile(ctx, "pornhub", "password"); return e }},
		{"register username", func() error { _, e := RegisterGameAccount(ctx, "pornhub", "password"); return e }},
		{"register nickname", func() error { _, e := RegisterGameAccount(ctx, "safeuser", "password", "色情"); return e }},
		{"global username", func() error { _, e := prepareGlobalGameAccount(ctx, "pornhub", "password", "玩家"); return e }},
		{"global nickname", func() error { _, e := prepareGlobalGameAccount(ctx, "safeuser", "password", "色情"); return e }},
		{"admin create name", func() error {
			_, e := game.AdminCreateGamePlayer(ctx, nil, model.AdminGamePlayerCreateRequest{AccountID: 1, Name: "色情"})
			return e
		}},
		{"admin create username", func() error {
			_, e := game.AdminCreateGamePlayer(ctx, nil, model.AdminGamePlayerCreateRequest{Username: "pornhub", Name: "玩家"})
			return e
		}},
		{"admin account nickname", func() error {
			_, e := AdminUpdateGlobalGameAccount(ctx, nil, 1, model.AdminGameAccountUpdateRequest{Nickname: "色情"})
			return e
		}},
		{"current account nickname", func() error { _, e := UpdateCurrentGlobalGameAccount(ctx, 1, nil, "色情", "", ""); return e }},
		{"player rename", func() error {
			_, e := game.changePlayerName(ctx, 1, &pb.CSChangePlayerName{PlayerName: proto.String("色情")}, 1)
			return e
		}},
		{"admin player rename", func() error {
			_, e := AdminRenameGamePlayer(ctx, nil, 1, model.AdminGamePlayerNameRequest{Name: "色情", Reason: "test"})
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, constUtil.ErrNameSensitive) {
				t.Fatalf("got %v, want sensitive name error", err)
			}
		})
	}
}

func TestContentModerationCreatePlayerRejectsBeforeWriteAndPreservesExisting(t *testing.T) {
	tx, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "names.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := tx.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if err := tx.AutoMigrate(&db.Profile{}); err != nil {
		t.Fatal(err)
	}
	row := db.Profile{Uid: "safeuser", Session: []byte{1}}
	if err := tx.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := createPlayerLocked(tx, row.Id, "色情", "hero"); !errors.Is(err, constUtil.ErrNameSensitive) {
		t.Fatalf("got %v", err)
	}
	var stored db.Profile
	if err := tx.First(&stored, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Player != nil || stored.Game != nil || stored.CreatedAt != nil {
		t.Fatal("rejected creation wrote player state")
	}
	existing := `{"name":"原名"}`
	if err := tx.Model(&row).Update("player", existing).Error; err != nil {
		t.Fatal(err)
	}
	got, err := createPlayerLocked(tx, row.Id, "色情", "hero")
	if err != nil || got.Player == nil || *got.Player != existing {
		t.Fatalf("existing role retry changed: %v, %v", got, err)
	}
}

func TestContentModerationLegacyAutoRegistrationAndExistingLogin(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "legacy.sqlite")); err != nil {
		t.Fatal(err)
	}
	connection, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if _, err := LoginAccount(ctx, "pornhub", "password"); !errors.Is(err, constUtil.ErrNameSensitive) {
		t.Fatalf("legacy auto-registration escaped: %v", err)
	}
	var count int64
	if err := db.GetIns().Model(&db.Profile{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rejected auto-registration created account", count, err)
	}
	// Seed an existing historical identity through an ordinary account, preserving
	// its valid credential. Passwords and existing login remain outside this rule.
	account, err := RegisterGameAccount(ctx, "safe-user", "色情-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", account.PlayerID).Update("uid", "pornhub").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := LoginAccount(ctx, "pornhub", "色情-password"); err != nil {
		t.Fatal("existing login was blocked", err)
	}
}
