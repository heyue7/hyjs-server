package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"hyjs-server/utils"
)

// TestGameAccountNewProfileUsesArgon2id 确认游戏注册/改密入口使用当前 PHC 凭据并保留独立随机 salt。
func TestGameAccountNewProfileUsesArgon2id(t *testing.T) {
	ctx := context.Background()
	password := "isolated-profile-password"
	first, err := gameAccountNewProfile(ctx, "profile-format-user", password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gameAccountNewProfile(ctx, "profile-format-user", password)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "$argon2id$v=19$m=19456,t=2,p=1$"
	if len(first.PasswordSalt) != 16 || len(first.PasswordHash) == 0 || !strings.HasPrefix(string(first.PasswordHash), prefix) || len(first.Session) != 16 {
		t.Fatalf("profile credential layout: salt=%d hash=%q session=%d", len(first.PasswordSalt), first.PasswordHash, len(first.Session))
	}
	if bytes.Equal(first.PasswordSalt, second.PasswordSalt) || bytes.Equal(first.PasswordHash, second.PasswordHash) {
		t.Fatal("profile creation reused salt or hash")
	}
	valid, legacy, err := utils.VerifyPasswordCredential(ctx, password, first.PasswordSalt, first.PasswordHash)
	if err != nil || !valid || legacy {
		t.Fatalf("profile credential verify = %t/%t/%v", valid, legacy, err)
	}
	valid, legacy, err = utils.VerifyPasswordCredential(ctx, password+"-wrong", first.PasswordSalt, first.PasswordHash)
	if err != nil || valid || legacy {
		t.Fatalf("profile wrong password verify = %t/%t/%v", valid, legacy, err)
	}
}
