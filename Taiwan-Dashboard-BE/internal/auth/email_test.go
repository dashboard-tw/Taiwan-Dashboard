package auth

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

func testVault(t *testing.T, seed byte) *EmailVault {
	t.Helper()
	v, err := NewEmailVault(bytes.Repeat([]byte{seed}, EmailKeySize))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEmailSealOpen(t *testing.T) {
	v := testVault(t, 1)
	a, err := v.Seal(7, "octo@example.com")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := v.Seal(7, "octo@example.com")
	if bytes.Equal(a, b) {
		t.Error("two encryptions of the same email are identical (nonce reused)")
	}
	if bytes.Contains(a, []byte("octo")) {
		t.Error("ciphertext contains the plaintext")
	}
	got, err := v.Open(7, a)
	if err != nil || got != "octo@example.com" {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

func TestEmailOpenRefuses(t *testing.T) {
	v := testVault(t, 1)
	sealed, _ := v.Seal(7, "octo@example.com")

	if _, err := v.Open(8, sealed); err != ErrEmailUnreadable {
		t.Errorf("another user's row: err = %v", err)
	}
	if _, err := testVault(t, 2).Open(7, sealed); err != ErrEmailUnreadable {
		t.Errorf("wrong key: err = %v", err)
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	if _, err := v.Open(7, tampered); err != ErrEmailUnreadable {
		t.Errorf("tampered: err = %v", err)
	}
	for _, bad := range [][]byte{nil, {emailFormat}, append([]byte{9}, sealed[1:]...)} {
		if _, err := v.Open(7, bad); err != ErrEmailUnreadable {
			t.Errorf("Open(%x) err = %v", bad, err)
		}
	}
}

func TestEmailFingerprint(t *testing.T) {
	v := testVault(t, 1)
	fp := v.Fingerprint(7, "Octo@Example.com ")
	if len(fp) != 32 {
		t.Fatalf("len = %d", len(fp))
	}
	if !v.Matches(7, "octo@example.com", fp) {
		t.Error("normalised email does not match")
	}
	if v.Matches(7, "other@example.com", fp) {
		t.Error("another email matches")
	}
	if v.Matches(8, "octo@example.com", fp) {
		t.Error("same email under another user id matches")
	}
	if bytes.Equal(fp, testVault(t, 2).Fingerprint(7, "octo@example.com")) {
		t.Error("fingerprint does not depend on the key")
	}
}

func TestEmailFingerprintDigest(t *testing.T) {
	v := testVault(t, 1)
	digest := sha256.Sum256([]byte("octo@example.com"))
	if !bytes.Equal(v.FingerprintDigest(7, digest[:]), v.Fingerprint(7, " Octo@Example.com")) {
		t.Error("digest of the normalised email does not give the same fingerprint")
	}
	if bytes.Equal(v.FingerprintDigest(8, digest[:]), v.Fingerprint(7, "octo@example.com")) {
		t.Error("digest matches another user id")
	}
}

func TestEmailLookup(t *testing.T) {
	v := testVault(t, 1)
	a := v.Lookup(" Octo@Example.com")
	if len(a) != 32 || !bytes.Equal(a, v.Lookup("octo@example.com")) {
		t.Error("lookup depends on case or spaces")
	}
	if bytes.Equal(a, v.Lookup("other@example.com")) {
		t.Error("two emails share a lookup value")
	}
	for id := int64(0); id < 10; id++ {
		if bytes.Equal(a, v.Fingerprint(id, "octo@example.com")) {
			t.Fatal("lookup equals a fingerprint")
		}
	}
	if bytes.Equal(a, testVault(t, 2).Lookup("octo@example.com")) {
		t.Error("lookup does not depend on the key")
	}
}

func TestEmailSealLimit(t *testing.T) {
	v := testVault(t, 1)
	if _, err := v.Seal(7, strings.Repeat("a", MaxSealLen)); err != nil {
		t.Errorf("a value at the limit was refused: %v", err)
	}
	if _, err := v.Seal(7, strings.Repeat("a", MaxSealLen+1)); !errors.Is(err, ErrSealTooLong) {
		t.Errorf("an oversized value was sealed: %v", err)
	}
}
