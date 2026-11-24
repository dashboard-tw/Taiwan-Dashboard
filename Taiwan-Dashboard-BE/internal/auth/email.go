package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const emailFormat = 1

const EmailKeySize = 32

var ErrEmailUnreadable = errors.New("stored email cannot be decrypted")
var ErrSealTooLong = errors.New("content is too long to encrypt")

const MaxSealLen = 4096

type EmailVault struct {
	aead      cipher.AEAD
	indexKey  []byte
	lookupKey []byte
}

func NewEmailVault(master []byte) (*EmailVault, error) {
	if len(master) != EmailKeySize {
		return nil, fmt.Errorf("email master key must be %d bytes, got %d", EmailKeySize, len(master))
	}
	encKey, err := hkdf.Key(sha256.New, master, nil, "dashboard.tw email encryption v1", 32)
	if err != nil {
		return nil, err
	}
	indexKey, err := hkdf.Key(sha256.New, master, nil, "dashboard.tw email fingerprint v1", 32)
	if err != nil {
		return nil, err
	}
	lookupKey, err := hkdf.Key(sha256.New, master, nil, "dashboard.tw email lookup v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &EmailVault{aead: aead, indexKey: indexKey, lookupKey: lookupKey}, nil
}

func (v *EmailVault) Lookup(email string) []byte {
	m := hmac.New(sha256.New, v.lookupKey)
	m.Write([]byte(NormalizeEmail(email)))
	return m.Sum(nil)
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func emailAAD(userID int64) []byte {
	return []byte("dashboard.tw email v1 user:" + strconv.FormatInt(userID, 10))
}

func (v *EmailVault) Seal(userID int64, email string) ([]byte, error) {
	if len(email) > MaxSealLen {
		return nil, ErrSealTooLong
	}
	out := make([]byte, 1+v.aead.NonceSize(), 1+v.aead.NonceSize()+len(email)+v.aead.Overhead())
	out[0] = emailFormat
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, err
	}
	return v.aead.Seal(out, out[1:], []byte(email), emailAAD(userID)), nil
}

func (v *EmailVault) Open(userID int64, sealed []byte) (string, error) {
	n := v.aead.NonceSize()
	if len(sealed) < 1+n+v.aead.Overhead() || sealed[0] != emailFormat {
		return "", ErrEmailUnreadable
	}
	plain, err := v.aead.Open(nil, sealed[1:1+n], sealed[1+n:], emailAAD(userID))
	if err != nil {
		return "", ErrEmailUnreadable
	}
	return string(plain), nil
}

func (v *EmailVault) Fingerprint(userID int64, email string) []byte {
	inner := sha256.Sum256([]byte(NormalizeEmail(email)))
	return v.FingerprintDigest(userID, inner[:])
}

func (v *EmailVault) FingerprintDigest(userID int64, digest []byte) []byte {
	m := hmac.New(sha256.New, v.indexKey)
	m.Write(digest)
	var id [8]byte
	binary.BigEndian.PutUint64(id[:], uint64(userID))
	m.Write(id[:])
	return m.Sum(nil)
}

func (v *EmailVault) Matches(userID int64, email string, fingerprint []byte) bool {
	return hmac.Equal(v.Fingerprint(userID, email), fingerprint)
}
