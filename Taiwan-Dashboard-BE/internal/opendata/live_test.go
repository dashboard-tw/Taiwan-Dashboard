package opendata

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

func TestQueriesAgainstLiveDatabase(t *testing.T) {
	url := os.Getenv("OPENDATA_LIVE_URL")
	if url == "" {
		t.Skip("OPENDATA_LIVE_URL not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var readOnly string
	if err := db.QueryRow(`SHOW default_transaction_read_only`).Scan(&readOnly); err != nil || readOnly != "on" {
		t.Fatalf("the connection must be read-only (default_transaction_read_only=on), got %q %v", readOnly, err)
	}
	svc := &Service{DB: db}
	ctx := context.Background()
	now := time.Now()

	for i := range Datasets {
		d := &Datasets[i]
		t.Run(d.ID, func(t *testing.T) {
			if d.HasSite {
				st, err := svc.Sites(ctx, d, nil)
				if err != nil {
					t.Fatalf("sites: %v", err)
				}
				t.Logf("sites %d", len(st.Rows))
			}
			lt, next, err := svc.Latest(ctx, d, nil, nil, "")
			if err != nil {
				t.Fatalf("latest: %v", err)
			}
			t.Logf("latest %d (next page %v)", len(lt.Rows), next != "")
			if next != "" {
				if _, _, err := svc.Latest(ctx, d, nil, nil, next); err != nil {
					t.Fatalf("latest page 2: %v", err)
				}
			}
			if d.HasCounty {
				county := "63000"
				if _, _, err := svc.Latest(ctx, d, &county, nil, ""); err != nil {
					t.Fatalf("latest by county: %v", err)
				}
			}
			if d.HasHistory() {
				ht, hnext, err := svc.History(ctx, d, HistoryQuery{From: now.Add(-MaxRange), To: now})
				if err != nil {
					t.Fatalf("history: %v", err)
				}
				t.Logf("history %d (next page %v)", len(ht.Rows), hnext != "")
				if hnext != "" {
					if _, _, err := svc.History(ctx, d, HistoryQuery{From: now.Add(-MaxRange), To: now, Cursor: hnext}); err != nil {
						t.Fatalf("history page 2: %v", err)
					}
				}
			}
			if d.Months != "" {
				rows, err := db.QueryContext(ctx, d.Months)
				if err != nil {
					t.Fatalf("months: %v", err)
				}
				n := 0
				for rows.Next() {
					var month string
					var count int64
					if err := rows.Scan(&month, &count); err != nil {
						t.Fatalf("months scan: %v", err)
					}
					n++
				}
				rows.Close()
				t.Logf("months %d", n)
			}
		})
	}
}
