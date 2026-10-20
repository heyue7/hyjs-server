package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	"hyjs-server/constUtil"
)

var (
	// ErrPasswordWorkBusy 表示密码计算槽位已满或等待超时，调用方应稍后重试而不是继续排队。
	ErrPasswordWorkBusy = errors.New("密码计算繁忙，请稍后重试")

	errPasswordCredentialFormat = errors.New("密码凭据格式无效")
)

// 所有密码计算共用固定槽位，避免大量并发登录或注册同时占用内存和 CPU。
var passwordCredentialSlots = make(chan struct{}, constUtil.PasswordCredentialSlots)

var passwordCredentialWaiters = struct {
	sync.Mutex
	waiting int
}{}

// acquirePasswordCredentialSlot 受 ctx、等待人数上限和最长等待时间约束，任何出口都会释放等待计数。
func acquirePasswordCredentialSlot(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	passwordCredentialWaiters.Lock()
	if passwordCredentialWaiters.waiting >= constUtil.PasswordCredentialMaxWaiters {
		passwordCredentialWaiters.Unlock()
		return nil, ErrPasswordWorkBusy
	}
	passwordCredentialWaiters.waiting++
	passwordCredentialWaiters.Unlock()
	leaveWait := func() {
		passwordCredentialWaiters.Lock()
		passwordCredentialWaiters.waiting--
		passwordCredentialWaiters.Unlock()
	}
	timer := time.NewTimer(constUtil.PasswordCredentialWait)
	defer timer.Stop()
	select {
	case passwordCredentialSlots <- struct{}{}:
		leaveWait()
		return func() { <-passwordCredentialSlots }, nil
	case <-ctx.Done():
		leaveWait()
		return nil, ctx.Err()
	case <-timer.C:
		leaveWait()
		return nil, ErrPasswordWorkBusy
	}
}

// NewPasswordCredential 使用当前 Argon2id 参数生成独立 salt 和包含参数的标准 PHC 编码。
func NewPasswordCredential(ctx context.Context, password string) (salt []byte, encoded []byte, err error) {
	if len(password) > constUtil.PasswordCredentialMaxBytes {
		return nil, nil, errPasswordCredentialFormat
	}
	release, err := acquirePasswordCredentialSlot(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	salt = make([]byte, constUtil.PasswordSaltBytes)
	if _, err = rand.Read(salt); err != nil {
		return nil, nil, err
	}
	digest := argon2.IDKey([]byte(password), salt, constUtil.PasswordArgon2Iterations, constUtil.PasswordArgon2MemoryKiB, constUtil.PasswordArgon2Threads, constUtil.PasswordArgon2KeyBytes)
	encoded = []byte(fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		constUtil.PasswordArgon2MemoryKiB,
		constUtil.PasswordArgon2Iterations,
		constUtil.PasswordArgon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	))
	return salt, encoded, nil
}

// VerifyPasswordCredential 识别当前 PHC、旧 scrypt 和旧后台 bcrypt；legacy 表示调用方应在成功后升级存储。
func VerifyPasswordCredential(ctx context.Context, password string, salt, encoded []byte) (valid bool, legacy bool, err error) {
	if len(password) > constUtil.PasswordCredentialMaxBytes {
		return false, false, errPasswordCredentialFormat
	}
	if len(encoded) == 0 || len(encoded) > 512 {
		return false, false, errPasswordCredentialFormat
	}
	release, err := acquirePasswordCredentialSlot(ctx)
	if err != nil {
		return false, false, err
	}
	defer release()

	if salt == nil {
		return verifyBcryptCredential(password, encoded)
	}
	if len(salt) > 64 {
		return false, false, errPasswordCredentialFormat
	}
	if bytes.HasPrefix(encoded, []byte("$argon2id$")) {
		return verifyArgon2Credential(password, salt, encoded)
	}
	if len(encoded) == constUtil.PasswordLegacyScryptKeyBytes {
		return verifyScryptCredential(password, salt, encoded)
	}
	return false, false, errPasswordCredentialFormat
}

func verifyArgon2Credential(password string, salt, encoded []byte) (bool, bool, error) {
	// 只接受本部署写出的规范 PHC 字段和参数，拒绝任意成本字段造成的内存放大。
	parts := strings.Split(string(encoded), "$")
	wantParameters := fmt.Sprintf("m=%d,t=%d,p=%d", constUtil.PasswordArgon2MemoryKiB, constUtil.PasswordArgon2Iterations, constUtil.PasswordArgon2Threads)
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != wantParameters {
		return false, false, errPasswordCredentialFormat
	}
	phcSalt, err := decodePasswordBase64(parts[4])
	if err != nil || len(phcSalt) != constUtil.PasswordSaltBytes || !bytes.Equal(phcSalt, salt) {
		return false, false, errPasswordCredentialFormat
	}
	wantHash, err := decodePasswordBase64(parts[5])
	if err != nil || len(wantHash) != constUtil.PasswordArgon2KeyBytes {
		return false, false, errPasswordCredentialFormat
	}
	digest := argon2.IDKey([]byte(password), phcSalt, constUtil.PasswordArgon2Iterations, constUtil.PasswordArgon2MemoryKiB, constUtil.PasswordArgon2Threads, constUtil.PasswordArgon2KeyBytes)
	return subtle.ConstantTimeCompare(digest, wantHash) == 1, false, nil
}

func verifyScryptCredential(password string, salt, encoded []byte) (bool, bool, error) {
	if len(salt) != constUtil.PasswordLegacyScryptSaltBytes {
		return false, false, errPasswordCredentialFormat
	}
	digest, err := scrypt.Key([]byte(password), salt, constUtil.PasswordLegacyScryptN, constUtil.PasswordLegacyScryptR, constUtil.PasswordLegacyScryptP, constUtil.PasswordLegacyScryptKeyBytes)
	if err != nil {
		return false, false, err
	}
	return subtle.ConstantTimeCompare(digest, encoded) == 1, true, nil
}

func verifyBcryptCredential(password string, encoded []byte) (bool, bool, error) {
	if len(encoded) > 256 {
		return false, false, errPasswordCredentialFormat
	}
	cost, err := bcrypt.Cost(encoded)
	if err != nil || cost < constUtil.PasswordLegacyBcryptMinCost || cost > constUtil.PasswordLegacyBcryptMaxCost {
		return false, false, errPasswordCredentialFormat
	}
	if bcrypt.CompareHashAndPassword(encoded, []byte(password)) != nil {
		return false, true, nil
	}
	return true, true, nil
}

// decodePasswordBase64 拒绝填充、非法字母表和非规范尾位，确保同一凭据只有一种编码。
func decodePasswordBase64(value string) ([]byte, error) {
	if value == "" || len(value) > 128 {
		return nil, errPasswordCredentialFormat
	}
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || base64.RawStdEncoding.EncodeToString(decoded) != value {
		return nil, errPasswordCredentialFormat
	}
	return decoded, nil
}
