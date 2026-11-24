package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/hooks"
	"dashboard.tw/backend/internal/mail"
)

type Notifier struct {
	Sender     mail.Sender
	Store      *auth.Store
	Site       string
	Interval   time.Duration
	DailyLimit int
	Sandbox    bool
	Hooks      *hooks.Dispatcher
	wg         sync.WaitGroup
	warned     sync.Map
	wake       chan struct{}
}

func New(sender mail.Sender, store *auth.Store, site string) *Notifier {
	return &Notifier{Sender: sender, Store: store, Site: site, wake: make(chan struct{}, 1)}
}

var taipei = time.FixedZone("Asia/Taipei", 8*3600)

func (n *Notifier) enabled() bool { return n != nil && n.Sender != nil && n.Store != nil }

func (n *Notifier) Enabled() bool { return n.enabled() }

func (n *Notifier) Wait() {
	if n != nil {
		n.wg.Wait()
	}
}

func (n *Notifier) SendTest(ctx context.Context, key, to string) error {
	if !n.enabled() {
		return errors.New("寄信功能沒有啟用")
	}
	t, err := load(ctx, n.Store.DB, key)
	if err != nil {
		return err
	}
	data := Sample(key)
	data["Site"] = n.Site
	r, err := t.renderAll(data)
	if err != nil {
		return err
	}
	return n.Sender.Send(ctx, mail.Message{To: to, Subject: "［測試］" + r.Subject, Text: r.Text, HTML: r.HTML, Tag: "test"})
}

func (n *Notifier) userInfo(ctx context.Context, userID int64) (email, name string, err error) {
	email, err = n.Store.UserEmail(ctx, userID)
	if err == nil {
		err = n.Store.DB.QueryRowContext(ctx, `SELECT display_name FROM app.users WHERE id = $1`, userID).Scan(&name)
	}
	return email, name, err
}

func (n *Notifier) toUser(key string, userID int64, data map[string]string) {
	if !n.enabled() {
		return
	}
	n.queue(job{template: key, userID: userID, data: data})
}

func providerName(p string) string {
	switch p {
	case "github":
		return "GitHub"
	case "google":
		return "Google"
	}
	return p
}

func clock(t time.Time) string { return t.In(taipei).Format("2006/01/02 15:04") }

func (n *Notifier) OnEvent(e auth.Event) {
	switch e.Kind {
	case auth.EventSignup:
		n.toUser("signup", e.UserID, map[string]string{"Provider": providerName(e.Provider)})
	case auth.EventNewDevice:
		n.toUser("new_device", e.UserID, clientVars(e, nil))
	case auth.EventLink, auth.EventUnlink:
		n.toUser(e.Kind, e.UserID, clientVars(e, map[string]string{"Provider": providerName(e.Provider)}))
	case auth.EventAPIKeyCreated, auth.EventAPIKeyRotated, auth.EventAPIKeyDisabled, auth.EventAPIKeyDeleted:
		n.toUser(e.Kind, e.UserID, clientVars(e, map[string]string{"Key": e.Key, "ClientID": e.ClientID}))
	}
}

func clientVars(e auth.Event, data map[string]string) map[string]string {
	if data == nil {
		data = map[string]string{}
	}
	d := auth.DescribeDevice(e.UserAgent)
	data["Time"] = clock(e.At)
	data["IP"] = orUnknown(e.IP)
	data["Location"] = orUnknown(e.Client.Location())
	data["DeviceType"] = orUnknown(d.Type)
	data["OS"] = orUnknown(d.OS)
	data["Browser"] = orUnknown(d.Browser)
	data["Device"] = auth.DeviceName(e.UserAgent)
	return data
}

func (n *Notifier) AccountDeleting(ctx context.Context, userID int64) func() {
	if !n.enabled() {
		return func() {}
	}
	email, name, err := n.userInfo(ctx, userID)
	var enc []byte
	if err == nil && email != "" && n.Store.Emails != nil {
		enc, err = n.Store.Emails.Seal(userID, email)
	}
	if err != nil {
		slog.Error("mail: account delete lookup", "user", userID, "error", err)
		return func() {}
	}
	if enc == nil {
		return func() {}
	}
	return func() {
		n.queue(job{template: "account_deleted", userID: userID, emailEnc: enc,
			data: map[string]string{"Name": name, "Time": clock(time.Now())}})
	}
}

func (n *Notifier) UsageHigh(userID int64, used auth.Usage, tier auth.Tier) {
	if !n.enabled() {
		return
	}
	if used.Requests*10 < tier.PerDay*8 && used.Bytes*10 < tier.BytesPerDay*8 {
		return
	}
	day := time.Now().In(taipei).Format(time.DateOnly)
	if v, ok := n.warned.Load(userID); ok && v == day {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := n.Store.MarkUsageWarned(ctx, userID)
	if err != nil {
		slog.Error("mail: mark usage warned", "user", userID, "error", err)
		return
	}
	n.warned.Store(userID, day)
	if !first {
		return
	}
	n.toUser("api_usage", userID, map[string]string{
		"Requests":       thousands(int64(used.Requests)),
		"RequestLimit":   thousands(int64(tier.PerDay)),
		"TrafficMB":      fmt.Sprintf("%.1f", float64(used.Bytes)/(1<<20)),
		"TrafficLimitMB": thousands(tier.BytesPerDay >> 20),
	})
}

func (n *Notifier) AppReviewed(userID int64, app, action, note string) {
	key := map[string]string{"approve": "app_approved", "reject": "app_rejected", "remove": "app_removed"}[action]
	if key == "" {
		return
	}
	n.toUser(key, userID, map[string]string{"App": app, "Note": note})
}

func (n *Notifier) AppSubmitted(userID int64, app, author string, updated bool) {
	if !n.enabled() {
		return
	}
	n.queue(job{template: "app_received", userID: userID, data: map[string]string{"App": app, "Author": author},
		mergeKey: fmt.Sprintf("app_received:%d", userID), delay: mergeDelay})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var ids []int64
	err := db.AsSystem(ctx, n.Store.DB, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM app.users WHERE role = 'admin' AND status = 'active'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		slog.Error("mail: list admins", "error", err)
		return
	}
	key := "app_submitted"
	if updated {
		key = "app_resubmitted"
	}
	for _, id := range ids {
		n.queue(job{template: key, userID: id, data: map[string]string{"App": app, "Author": author},
			mergeKey: fmt.Sprintf("%s:%d", key, id), delay: mergeDelay})
	}
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "不明"
	}
	return s
}

func thousands(v int64) string {
	s := fmt.Sprint(v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func (n *Notifier) APIKeysRevoked(userID int64, names []string) {
	for _, name := range names {
		n.toUser("api_key_revoked", userID, map[string]string{"Key": name, "Time": clock(time.Now())})
	}
}
