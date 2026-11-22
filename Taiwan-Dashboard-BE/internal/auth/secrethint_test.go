package auth

import (
	"strings"
	"testing"
)

func TestSecretHint(t *testing.T) {
	secret, err := newClientSecret()
	if err != nil {
		t.Fatal(err)
	}
	h := secretHint(secret)
	if !strings.HasPrefix(h, secret[:8]+"-") || !strings.HasSuffix(h, "-"+secret[len(secret)-8:]) {
		t.Errorf("hint %q does not show the ends of %q", h, secret)
	}
	hex := strings.ReplaceAll(secret, "-", "")
	shown := strings.ReplaceAll(secret[:8]+secret[len(secret)-8:], "-", "")
	if len(hex)-len(shown) < 32 {
		t.Errorf("only %d hidden hex characters", len(hex)-len(shown))
	}
	if strings.Contains(h, secret[9:17]) {
		t.Errorf("hint %q leaks the middle of the secret", h)
	}
	if secretHint("short") != "" {
		t.Error("hint for a malformed secret")
	}
}
