package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/auth"
)

func newUserAdminEnv(t *testing.T) (e authEnv, admin string, userID int64) {
	t.Helper()
	e = newAdminEnv(t)
	vault, err := auth.NewEmailVault(make([]byte, auth.EmailKeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.store.Emails = vault
	admin = e.signInAs(t, auth.RoleAdmin, "active")
	if err := e.db.QueryRow(`INSERT INTO app.users (display_name) VALUES ('小明') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	enc, err := vault.Seal(userID, "ming@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE app.users SET email_enc = $2, email_hash = $3 WHERE id = $1`,
		userID, enc, vault.Fingerprint(userID, "ming@example.com")); err != nil {
		t.Fatal(err)
	}
	return e, admin, userID
}

func emailDigest(email string) string {
	d := sha256.Sum256([]byte(auth.NormalizeEmail(email)))
	return hex.EncodeToString(d[:])
}

func TestAdminSearchUserByEmailDigest(t *testing.T) {
	e, admin, userID := newUserAdminEnv(t)

	w := e.send("POST", "/api/v1/admin/users/search", admin, map[string]string{"email_sha256": emailDigest(" Ming@Example.com")})
	if w.Code != http.StatusOK {
		t.Fatalf("search: %d %s", w.Code, w.Body)
	}
	found := decode[[]userRow](t, w)
	if len(found) != 1 || found[0].ID != userID || found[0].DisplayName != "小明" {
		t.Fatalf("found = %+v", found)
	}

	w = e.send("POST", "/api/v1/admin/users/search", admin, map[string]string{"email_sha256": emailDigest("other@example.com")})
	if got := decode[[]userRow](t, w); w.Code != http.StatusOK || len(got) != 0 {
		t.Errorf("unknown email: %d %+v", w.Code, got)
	}
	for _, bad := range []string{"", "ming@example.com", "abcd"} {
		if w := e.send("POST", "/api/v1/admin/users/search", admin, map[string]string{"email_sha256": bad}); w.Code != http.StatusBadRequest {
			t.Errorf("digest %q: %d, want 400", bad, w.Code)
		}
	}
}

func TestAdminSearchWithoutEmailKey(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	w := e.send("POST", "/api/v1/admin/users/search", admin, map[string]string{"email_sha256": emailDigest("a@b.c")})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("no email key: %d, want 503", w.Code)
	}
}

func TestAdminUserDetailHasNoEmail(t *testing.T) {
	e, admin, userID := newUserAdminEnv(t)
	w := e.send("GET", "/api/v1/admin/users/"+itoa(userID), admin, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "ming@example.com") {
		t.Error("detail leaks the email")
	}
	d := decode[map[string]any](t, w)
	if d["has_email"] != true || d["is_me"] != false {
		t.Errorf("detail = %v", d)
	}
	if w := e.send("GET", "/api/v1/admin/users/999999", admin, nil); w.Code != http.StatusNotFound {
		t.Errorf("missing user: %d, want 404", w.Code)
	}
}

func TestAdminPromoteAndDisableUser(t *testing.T) {
	e, admin, userID := newUserAdminEnv(t)
	path := "/api/v1/admin/users/" + itoa(userID)

	if w := e.send("PATCH", path, admin, map[string]string{"role": "admin"}); w.Code != http.StatusNoContent {
		t.Fatalf("promote: %d %s", w.Code, w.Body)
	}
	if n := e.count(t, `SELECT count(*) FROM app.users WHERE id = $1 AND role = 'admin'`, userID); n != 1 {
		t.Error("user was not promoted")
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'admin.user.update' AND target = $1`, "user:"+itoa(userID)); n != 1 {
		t.Errorf("audit entries = %d", n)
	}

	session, _, err := e.store.CreateSession(t.Context(), userID, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if w := e.send("PATCH", path, admin, map[string]string{"status": "disabled"}); w.Code != http.StatusNoContent {
		t.Fatalf("disable: %d %s", w.Code, w.Body)
	}
	if w := e.send("GET", "/api/v1/me", session, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("disabled user's session still works: %d", w.Code)
	}

	for _, body := range []map[string]string{{"role": "owner"}, {"status": "gone"}, {}} {
		if w := e.send("PATCH", path, admin, body); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d, want 400", body, w.Code)
		}
	}
}

func TestAdminCannotChangeSelf(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	var id int64
	if err := e.db.QueryRow(`SELECT max(id) FROM app.users`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if w := e.send("PATCH", "/api/v1/admin/users/"+itoa(id), admin, map[string]string{"role": "user"}); w.Code != http.StatusForbidden {
		t.Errorf("demote self: %d, want 403", w.Code)
	}
}
