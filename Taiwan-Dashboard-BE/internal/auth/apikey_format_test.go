package auth

import (
	"regexp"
	"testing"
)

var (
	clientIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}$`)
	secretRe   = regexp.MustCompile(`^([0-9a-f]{8}-){5}[0-9a-f]{8}$`)
)

func TestClientIDFormat(t *testing.T) {
	a1, err := newClientID(7)
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := newClientID(7)
	b, _ := newClientID(8)
	for _, id := range []string{a1, a2, b} {
		if !clientIDRe.MatchString(id) {
			t.Errorf("client id %q does not match <user hash>-xxxx-xxxx-xxxx", id)
		}
	}
	if a1[:8] != a2[:8] {
		t.Errorf("same account, different user hash: %q %q", a1, a2)
	}
	if a1[:8] == b[:8] {
		t.Errorf("different accounts share a user hash: %q %q", a1, b)
	}
	if a1 == a2 {
		t.Error("two client ids are identical")
	}
}

func TestClientSecretFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := newClientSecret()
		if err != nil {
			t.Fatal(err)
		}
		if !secretRe.MatchString(s) {
			t.Fatalf("secret %q does not match six dashed groups of hex", s)
		}
		if seen[s] {
			t.Fatal("a secret repeated")
		}
		seen[s] = true
	}
}

func TestDashed(t *testing.T) {
	for in, want := range map[string]string{
		"abcdefgh":     "abcd-efgh",
		"abcdefghijkl": "abcd-efgh-ijkl",
		"abc":          "abc",
		"abcde":        "abcd-e",
	} {
		if got := dashed(in, 4); got != want {
			t.Errorf("dashed(%q) = %q, want %q", in, got, want)
		}
	}
}
