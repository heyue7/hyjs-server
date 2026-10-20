package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"hyjs-server/constUtil"
)

// VerifyUnavailablePassword pays the normal bounded password-work cost for an
// unknown identity without generating a new credential or revealing existence.
func VerifyUnavailablePassword(ctx context.Context, password string) error {
	salt := make([]byte, constUtil.PasswordSaltBytes)
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", constUtil.PasswordArgon2MemoryKiB, constUtil.PasswordArgon2Iterations, constUtil.PasswordArgon2Threads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	_, _, err := VerifyPasswordCredential(ctx, password, salt, []byte(encoded))
	return err
}
