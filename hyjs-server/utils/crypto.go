package utils

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	mrand "math/rand"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

func Base64Encode(buf []byte) string {
	return base64.StdEncoding.EncodeToString(buf)
}

func Base64Decode(str string) []byte {
	var count = (4 - len(str)%4) % 4
	str += strings.Repeat("=", count)
	buf, _ := base64.StdEncoding.DecodeString(str)
	return buf
}

func SHAWithRSA(key, data []byte) (string, error) {
	pkey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}

	h := crypto.Hash.New(crypto.SHA1)
	h.Write(data)
	hashed := h.Sum(nil)

	buf, err := rsa.SignPKCS1v15(rand.Reader, pkey.(*rsa.PrivateKey), crypto.SHA1, hashed)
	if err != nil {
		return "", err
	}
	return Base64Encode(buf), nil
}

func NewSalt() string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return ""
	}

	return base64.RawStdEncoding.EncodeToString(salt)
}

type Argon2Options struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
}

func Argon2(password, saltBase64 string) (string, error) {
	p := &Argon2Options{
		Time:    1,
		Memory:  64 * 1024,
		Threads: 4,
		KeyLen:  32,
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltBase64)
	if err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)

	encoded := fmt.Sprintf("$argon2id$v=19$t=%d$m=%d$p=%d$%s",
		p.Time, p.Memory, p.Threads,
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encoded, nil
}

func VerifyArgon2(password, encodedHash, saltBase64 string) (bool, error) {
	var (
		version      int
		time, memory uint32
		threads      uint8
		hashBase64   string
	)

	parts := strings.Split(encodedHash, "$")
	if len(parts) < 6 {
		return false, errors.New("invalid argon2 hash format")
	}

	if parts[1] != "argon2id" {
		return false, errors.New("invalid argon2 algorithm")
	}

	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false, errors.New("invalid version format")
	}
	if version != 19 {
		return false, fmt.Errorf("unsupported argon2 version: %d", version)
	}

	_, err = fmt.Sscanf(parts[3], "t=%d", &time)
	if err != nil {
		return false, errors.New("invalid time parameter")
	}
	_, err = fmt.Sscanf(parts[4], "m=%d", &memory)
	if err != nil {
		return false, errors.New("invalid memory parameter")
	}
	_, err = fmt.Sscanf(parts[5], "p=%d", &threads)
	if err != nil {
		return false, errors.New("invalid threads parameter")
	}

	if len(parts) < 7 || parts[6] == "" {
		return false, errors.New("missing hash in encoded format")
	}
	hashBase64 = parts[6]

	if time == 0 || time > 100 {
		return false, errors.New("time parameter out of valid range")
	}
	if memory == 0 || memory > 1024*1024 {
		return false, errors.New("memory parameter out of valid range")
	}
	if threads == 0 || threads > 16 {
		return false, errors.New("threads parameter out of valid range")
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltBase64)
	if err != nil {
		return false, err
	}
	originalHash, err := base64.RawStdEncoding.DecodeString(hashBase64)
	if err != nil {
		return false, fmt.Errorf("invalid hash format: %w", err)
	}
	if len(originalHash) == 0 {
		return false, errors.New("hash cannot be empty after decoding")
	}

	const expectedKeyLen = 32
	if len(originalHash) != expectedKeyLen {
		return false, errors.New("hash length mismatch")
	}

	newHash := argon2.IDKey([]byte(password), salt, time, memory, threads, expectedKeyLen)

	if !bytes.Equal(newHash, originalHash) {
		return false, nil
	}
	return true, nil
}

func MaskRandomChars(s string) string {
	runes := []rune(s)
	n := len(runes)

	if n <= 1 {
		return s
	}

	r := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	count := r.Intn(n-1) + 1

	replaceIndex := make(map[int]struct{})

	for len(replaceIndex) < count {
		idx := r.Intn(n)
		if _, exists := replaceIndex[idx]; !exists {
			runes[idx] = '*'
			replaceIndex[idx] = struct{}{}
		}
	}

	return string(runes)
}

func ValidatePassword(pwd string) (status int) {
	if len(pwd) < 6 {
		return 1
	}

	if strings.ContainsFunc(pwd, unicode.IsSpace) {
		return 2
	}

	hasUpper := strings.ContainsFunc(pwd, unicode.IsUpper)
	hasLower := strings.ContainsFunc(pwd, unicode.IsLower)
	hasDigit := strings.ContainsFunc(pwd, unicode.IsDigit)
	hasSpecial := strings.ContainsFunc(pwd, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	count := 0
	for _, has := range []bool{hasUpper, hasLower, hasDigit, hasSpecial} {
		if has {
			count++
		}
	}

	if count < 2 {
		return 3
	}

	return 0
}

func IsEmailValid(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

func IsPhoneValid(phone string) bool {
	re := regexp.MustCompile(`^1[3-9]\d{9}$`)
	return re.MatchString(phone)
}

func GetTrimmedCell(row []string, cellIndex int) string {
	if cellIndex < 0 || cellIndex >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[cellIndex])
}

var markdownImageWithOptionalPrevNewlineRe = regexp.MustCompile(`(?:\r?\n)?!\[[^\]]*\]\([^)]*\)`)

func RemoveMarkdownImageLinks(s string) string {
	if s == "" {
		return s
	}
	return markdownImageWithOptionalPrevNewlineRe.ReplaceAllString(s, "")
}
