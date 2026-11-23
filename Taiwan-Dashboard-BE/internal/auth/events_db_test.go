package auth_test

import (
	"context"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/testdb"
)

const (
	winChrome130 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	winChrome131 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	iPhone       = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
)

func TestAccountEvents(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	var got []string
	s := &auth.Store{DB: db, OnEvent: func(e auth.Event) { got = append(got, e.Kind+":"+e.Provider) }}
	ctx := context.Background()

	u, _, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "1", Name: "Octo"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ua := range []string{winChrome130, winChrome131, iPhone, iPhone} {
		if _, _, err := s.CreateSession(ctx, u.ID, "203.0.113.1", ua); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LinkIdentity(ctx, u.ID, "google", "g1", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkIdentity(ctx, u.ID, "google", "g1", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UnlinkIdentity(ctx, u.ID, "google", ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"signup:github", "new_device:", "link:google", "unlink:google"}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("events = %v, want %v", got, want)
			break
		}
	}
}

func TestMarkUsageWarned(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	s := &auth.Store{DB: db}
	ctx := context.Background()
	u, _, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "2", Name: "Dev"})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		first, err := s.MarkUsageWarned(ctx, u.ID)
		if err != nil || first != want {
			t.Errorf("call %d: first = %v, %v; want %v", i+1, first, err, want)
		}
	}
}

func TestAPIKeyEvents(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	var got []string
	s := &auth.Store{DB: db, OnEvent: func(e auth.Event) {
		if e.Kind != auth.EventSignup {
			got = append(got, e.Kind+":"+e.Key)
			if e.ClientID == "" {
				t.Errorf("%s has no client id", e.Kind)
			}
		}
	}}
	ctx := context.Background()
	u, _, err := s.SignInExternal(ctx, "github", auth.Identity{Subject: "k1", Name: "Dev"})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.CreateClient(ctx, u.ID, "天氣 App", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetClientEnabled(ctx, u.ID, c.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClientEnabled(ctx, u.ID, c.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClient(ctx, u.ID, c.ID, ""); err != nil {
		t.Fatal(err)
	}
	want := "api_key_created:天氣 App,api_key_disabled:天氣 App,api_key_deleted:天氣 App"
	if strings.Join(got, ",") != want {
		t.Errorf("events = %v, want %s", got, want)
	}
}
