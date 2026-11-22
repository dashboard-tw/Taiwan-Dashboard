package auth_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/testdb"
)

func TestSignInExternalEmail(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	vault, err := auth.NewEmailVault(bytes.Repeat([]byte{3}, auth.EmailKeySize))
	if err != nil {
		t.Fatal(err)
	}
	s := &auth.Store{DB: db, Emails: vault}
	ctx := context.Background()

	u, created, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "4242", Name: "Octo", Email: "Octo@Example.com"})
	if err != nil || !created {
		t.Fatalf("first sign-in: created=%v err=%v", created, err)
	}

	var enc, fp []byte
	if err := db.QueryRow(`SELECT email_enc, email_hash FROM app.users WHERE id = $1`, u.ID).Scan(&enc, &fp); err != nil {
		t.Fatal(err)
	}
	if len(fp) != 32 || bytes.Contains(bytes.ToLower(enc), []byte("octo")) {
		t.Fatalf("stored enc=%x fp=%x", enc, fp)
	}
	if got, err := s.UserEmail(ctx, u.ID); err != nil || got != "Octo@Example.com" {
		t.Errorf("UserEmail = %q, %v", got, err)
	}
	if ok, err := s.EmailBelongsTo(ctx, u.ID, "octo@example.com"); err != nil || !ok {
		t.Errorf("EmailBelongsTo(own) = %v, %v", ok, err)
	}
	if ok, _ := s.EmailBelongsTo(ctx, u.ID, "someone@example.com"); ok {
		t.Error("EmailBelongsTo(other) = true")
	}

	if _, _, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "4242", Email: "octo@example.com"}); err != nil {
		t.Fatal(err)
	}
	var enc2 []byte
	_ = db.QueryRow(`SELECT email_enc FROM app.users WHERE id = $1`, u.ID).Scan(&enc2)
	if !bytes.Equal(enc, enc2) {
		t.Error("unchanged email was rewritten")
	}

	if _, _, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "4242", Email: "new@example.com"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UserEmail(ctx, u.ID); got != "new@example.com" {
		t.Errorf("after change UserEmail = %q", got)
	}

	_, _, err = s.SignInExternal(ctx, "google", auth.Identity{Subject: "g-octo", Email: "NEW@example.com"})
	var need *auth.LinkRequired
	if !errors.As(err, &need) || need.UserID != u.ID {
		t.Fatalf("same email via Google: err = %v, want LinkRequired for user %d", err, u.ID)
	}
	tok := auth.HashToken("link-token")
	if err := s.SavePendingLink(ctx, auth.PendingLink{TokenHash: tok, UserID: u.ID, Provider: "google", Subject: "g-octo", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if info, err := s.PendingLinkInfo(ctx, tok); err != nil || info.Provider != "google" || len(info.Existing) != 1 || info.Existing[0] != "github" {
		t.Fatalf("PendingLinkInfo = %+v, %v", info, err)
	}
	pl, err := s.TakePendingLink(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakePendingLink(ctx, tok); !errors.Is(err, auth.ErrNoSession) {
		t.Error("pending link taken twice")
	}
	if err := s.LinkIdentity(ctx, pl.UserID, pl.Provider, pl.Subject, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkIdentity(ctx, pl.UserID, pl.Provider, pl.Subject, ""); err != nil {
		t.Errorf("linking twice to the same account: %v", err)
	}
	other, _, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "other-user", Email: "other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LinkIdentity(ctx, other.ID, pl.Provider, pl.Subject, ""); !errors.Is(err, auth.ErrIdentityTaken) {
		t.Errorf("linking another account's identity: %v, want ErrIdentityTaken", err)
	}
	if got, created, err := s.SignInExternal(ctx, "google", auth.Identity{Subject: "g-octo", Email: "new@example.com"}); err != nil || created || got.ID != u.ID {
		t.Errorf("Google after linking: %+v created=%v err=%v", got, created, err)
	}

	if err := s.LinkIdentity(ctx, u.ID, "google", "g-second", ""); !errors.Is(err, auth.ErrProviderLinked) {
		t.Errorf("second Google: %v", err)
	}
	if err := s.UnlinkIdentity(ctx, u.ID, "google", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UnlinkIdentity(ctx, u.ID, "google", ""); !errors.Is(err, auth.ErrIdentityNotFound) {
		t.Errorf("unlink twice: %v", err)
	}
	if err := s.UnlinkIdentity(ctx, u.ID, "github", ""); !errors.Is(err, auth.ErrLastIdentity) {
		t.Errorf("unlink the last sign-in: %v", err)
	}

	plain := &auth.Store{DB: db}
	u2, _, err := plain.SignInExternal(ctx, "google", auth.Identity{Subject: "g-1", Email: "ming@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM app.users WHERE id = $1 AND email_enc IS NULL AND email_hash IS NULL`, u2.ID).Scan(&n)
	if n != 1 {
		t.Error("email stored without a vault")
	}

	var cols string
	_ = db.QueryRow(`SELECT string_agg(column_name, ',') FROM information_schema.columns WHERE table_schema = 'app' AND table_name = 'users'`).Scan(&cols)
	if strings.Contains(","+cols+",", ",email,") {
		t.Errorf("app.users still has a plaintext email column: %s", cols)
	}
}
