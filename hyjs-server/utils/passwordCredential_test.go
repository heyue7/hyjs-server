package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	"hyjs-server/constUtil"
)

func decodedPHCPart(t *testing.T, encoded string, index int) []byte {
	t.Helper()
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		t.Fatalf("unexpected PHC field count: %q", encoded)
	}
	value, err := base64.RawStdEncoding.DecodeString(parts[index])
	if err != nil {
		t.Fatalf("decode PHC field %d: %v", index, err)
	}
	return value
}

// TestPasswordCredentialFormatAndVerification 固定 PHC 字段、独立 salt 一致性和错误密码结果。
func TestPasswordCredentialFormatAndVerification(t *testing.T) {
	ctx := context.Background()
	password := "isolated-credential-password"
	salt, encoded, err := NewPasswordCredential(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if len(salt) != constUtil.PasswordSaltBytes {
		t.Fatalf("salt length = %d", len(salt))
	}
	prefix := "$argon2id$v=19$m=19456,t=2,p=1$"
	if !strings.HasPrefix(string(encoded), prefix) {
		t.Fatalf("non-canonical PHC prefix: %q", encoded)
	}
	phcSalt := decodedPHCPart(t, string(encoded), 4)
	wantHash := decodedPHCPart(t, string(encoded), 5)
	if !bytes.Equal(phcSalt, salt) {
		t.Fatal("independent salt differs from PHC salt")
	}
	if len(wantHash) != constUtil.PasswordArgon2KeyBytes {
		t.Fatalf("hash length = %d", len(wantHash))
	}
	valid, legacy, err := VerifyPasswordCredential(ctx, password, salt, encoded)
	if err != nil || !valid || legacy {
		t.Fatalf("current credential verify = %t/%t/%v", valid, legacy, err)
	}
	valid, legacy, err = VerifyPasswordCredential(ctx, password+"-wrong", salt, encoded)
	if err != nil || valid || legacy {
		t.Fatalf("wrong password verify = %t/%t/%v", valid, legacy, err)
	}
	// 同一明文必须使用随机 salt，重复注册/改密不能产生相同摘要。
	otherSalt, otherEncoded, err := NewPasswordCredential(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(otherSalt, salt) || bytes.Equal(otherEncoded, encoded) {
		t.Fatal("password credential reused salt or hash")
	}
}

// TestPasswordCredentialAcceptsLegacyScryptAndBcrypt 保留旧游戏 scrypt 与旧后台 bcrypt 的验证能力。
func TestPasswordCredentialAcceptsLegacyScryptAndBcrypt(t *testing.T) {
	ctx := context.Background()
	password := "legacy-isolated-password"
	salt := bytes.Repeat([]byte{0x2a}, constUtil.PasswordLegacyScryptSaltBytes)
	digest, err := scrypt.Key([]byte(password), salt, constUtil.PasswordLegacyScryptN, constUtil.PasswordLegacyScryptR, constUtil.PasswordLegacyScryptP, constUtil.PasswordLegacyScryptKeyBytes)
	if err != nil {
		t.Fatal(err)
	}
	valid, legacy, err := VerifyPasswordCredential(ctx, password, salt, digest)
	if err != nil || !valid || !legacy {
		t.Fatalf("scrypt verify = %t/%t/%v", valid, legacy, err)
	}
	valid, legacy, err = VerifyPasswordCredential(ctx, password+"-wrong", salt, digest)
	if err != nil || valid || !legacy {
		t.Fatalf("scrypt wrong verify = %t/%t/%v", valid, legacy, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	valid, legacy, err = VerifyPasswordCredential(ctx, password, nil, hash)
	if err != nil || !valid || !legacy {
		t.Fatalf("bcrypt verify = %t/%t/%v", valid, legacy, err)
	}
	valid, legacy, err = VerifyPasswordCredential(ctx, password+"-wrong", nil, hash)
	if err != nil || valid || !legacy {
		t.Fatalf("bcrypt wrong verify = %t/%t/%v", valid, legacy, err)
	}
}

// TestPasswordCredentialRejectsBadFormats 拒绝不匹配 salt、非规范参数和越界成本。
func TestPasswordCredentialRejectsBadFormats(t *testing.T) {
	ctx := context.Background()
	salt, encoded, err := NewPasswordCredential(ctx, "format-guard-password")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		password string
		salt     []byte
		encoded  []byte
	}{
		{name: "mismatched salt", password: "format-guard-password", salt: bytes.Repeat([]byte{0x01}, len(salt)), encoded: encoded},
		{name: "wrong memory", password: "format-guard-password", salt: salt, encoded: []byte(strings.Replace(string(encoded), "m=19456", "m=65536", 1))},
		{name: "wrong iterations", password: "format-guard-password", salt: salt, encoded: []byte(strings.Replace(string(encoded), "t=2", "t=9", 1))},
		{name: "unsupported version", password: "format-guard-password", salt: salt, encoded: []byte(strings.Replace(string(encoded), "$argon2id$v=19$", "$argon2id$v=20$", 1))},
		{name: "short scrypt digest", password: "format-guard-password", salt: salt, encoded: []byte{0x01}},
		{name: "scrypt wrong salt size", password: "format-guard-password", salt: []byte{0x01, 0x02}, encoded: bytes.Repeat([]byte{0x03}, constUtil.PasswordLegacyScryptKeyBytes)},
		{name: "bcrypt with salt", password: "format-guard-password", salt: salt, encoded: []byte("$2a$10$invalid")},
		{name: "oversized encoded", password: "format-guard-password", salt: salt, encoded: bytes.Repeat([]byte{0x41}, 513)},
	}
	for _, tc := range cases {
		valid, _, err := VerifyPasswordCredential(ctx, tc.password, tc.salt, tc.encoded)
		if err == nil || valid {
			t.Fatalf("%s accepted: valid=%t err=%v", tc.name, valid, err)
		}
	}
	lowCost, err := bcrypt.GenerateFromPassword([]byte("format-guard-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if valid, _, err := VerifyPasswordCredential(ctx, "format-guard-password", nil, lowCost); err == nil || valid {
		t.Fatalf("bcrypt below minimum cost accepted: valid=%t err=%v", valid, err)
	}
}

// TestPasswordCredentialCancellationAndBounds 验证取消退出、槽位满和等待人数上限都能快速失败。
func TestPasswordCredentialCancellationAndBounds(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := NewPasswordCredential(cancelled, "cancelled-password"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled NewPasswordCredential err = %v", err)
	}
	salt, encoded, err := NewPasswordCredential(context.Background(), "bounds-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyPasswordCredential(cancelled, "bounds-password", salt, encoded); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled VerifyPasswordCredential err = %v", err)
	}

	passwordCredentialSlots <- struct{}{}
	passwordCredentialSlots <- struct{}{}
	defer func() {
		passwordCredentialWaiters.Lock()
		passwordCredentialWaiters.waiting = 0
		passwordCredentialWaiters.Unlock()
		for i := 0; i < 2; i++ {
			select {
			case <-passwordCredentialSlots:
			default:
			}
		}
	}()
	passwordCredentialWaiters.Lock()
	passwordCredentialWaiters.waiting = constUtil.PasswordCredentialMaxWaiters
	passwordCredentialWaiters.Unlock()
	if _, _, err := NewPasswordCredential(context.Background(), "busy-password"); !errors.Is(err, ErrPasswordWorkBusy) {
		t.Fatalf("full waiter table err = %v", err)
	}
	passwordCredentialWaiters.Lock()
	passwordCredentialWaiters.waiting = 0
	passwordCredentialWaiters.Unlock()
	<-passwordCredentialSlots
	<-passwordCredentialSlots

	passwordCredentialSlots <- struct{}{}
	passwordCredentialSlots <- struct{}{}
	waiting, waitCancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := NewPasswordCredential(waiting, "waiting-password")
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		passwordCredentialWaiters.Lock()
		count := passwordCredentialWaiters.waiting
		passwordCredentialWaiters.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			waitCancel()
			<-done
			<-passwordCredentialSlots
			<-passwordCredentialSlots
			t.Fatal("waiting request did not register")
		}
		time.Sleep(time.Millisecond)
	}
	waitCancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting request cancellation err = %v", err)
	}
	<-passwordCredentialSlots
	<-passwordCredentialSlots
}
