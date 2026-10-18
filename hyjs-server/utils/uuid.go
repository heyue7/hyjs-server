package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"github.com/speps/go-hashids"
)

func UnsignedUUID() string {
	id := uuid.New()
	return strings.ReplaceAll(id.String(), "-", "")
}

func InviteCode(uid int64) string {
	hd := hashids.NewData()
	hd.Salt = "inviteCode"
	hd.MinLength = 6
	h, _ := hashids.NewWithData(hd)
	code, _ := h.EncodeInt64([]int64{uid})
	return code
}

func VerificationCode() string {
	maxValue := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, maxValue)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%06d", n.Int64())
}
