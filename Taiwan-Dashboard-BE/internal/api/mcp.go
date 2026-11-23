package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/mcpserver"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/screen"
)

type mcpMeter struct {
	store  *auth.Store
	limits *keyLimits
}

func (m mcpMeter) Charge(ctx context.Context, req *mcp.CallToolRequest, bytes int) error {
	var owner *auth.KeyOwner
	if req != nil && req.Extra != nil && req.Extra.TokenInfo != nil {
		owner, _ = req.Extra.TokenInfo.Extra["owner"].(*auth.KeyOwner)
	}
	if owner == nil {
		return errors.New("this tool needs an API credential")
	}
	ctx = context.WithoutCancel(ctx)
	reject := func() {
		if err := m.store.RejectAPIKey(ctx, owner); err != nil {
			slog.Error("count rejected mcp call", "error", err)
		}
	}
	if ok, wait := m.limits.allow(owner); !ok {
		reject()
		return fmt.Errorf("per-minute API limit (%d) reached; retry in %d seconds", owner.Tier.PerMinute, int(wait.Seconds())+1)
	}
	used, ok, err := m.store.UseAPIKey(ctx, owner)
	if err != nil {
		return err
	}
	if !ok {
		reject()
		if used.Bytes >= owner.Tier.BytesPerDay {
			return fmt.Errorf("today's API traffic (%d MB) is used up; it resets at midnight Taiwan time. search, list_datasets and describe_dataset still work", owner.Tier.BytesPerDay>>20)
		}
		return fmt.Errorf("today's API requests (%d) are used up; they reset at midnight Taiwan time. search, list_datasets and describe_dataset still work", owner.Tier.PerDay)
	}
	if err := m.store.AddTraffic(ctx, owner, int64(bytes)); err != nil {
		slog.Error("count mcp traffic", "error", err)
	}
	m.limits.notify.UsageHigh(owner.UserID, auth.Usage{Requests: used.Requests, Bytes: used.Bytes + int64(bytes)}, owner.Tier)
	return nil
}

func registerMCP(r *gin.Engine, store *auth.Store, open *opendata.Service, scr *screen.Service, limits *keyLimits,
	spec opendata.SpecOptions, site, version string) {
	openAPI := func(ctx context.Context) map[string]any {
		o := spec
		o.Names = map[string]string{}
		if list, err := open.List(ctx); err == nil {
			for _, s := range list {
				o.Names[s.ID] = s.Name
			}
		}
		return opendata.OpenAPI(o)
	}
	srv := mcpserver.New(mcpserver.Deps{Open: open, Screen: scr, Site: site, Version: version,
		Meter: mcpMeter{store: store, limits: limits}, OpenAPI: openAPI})
	h := mcpserver.Handler(srv, mcpserver.HTTPOptions{
		Verify: func(ctx context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
			var o *auth.KeyOwner
			var err error
			if id, secret, ok := strings.Cut(token, ":"); ok {
				o, err = store.LookupClient(ctx, id, secret)
			} else {
				o, err = store.LookupMCPToken(ctx, token)
			}
			if errors.Is(err, auth.ErrNoKey) || errors.Is(err, auth.ErrInvalidClient) {
				return nil, mcpauth.ErrInvalidToken
			}
			if err != nil {
				return nil, err
			}
			if err := store.TouchKey(ctx, o.KeyID); err != nil {
				slog.Error("touch api key", "error", err)
			}
			return &mcpauth.TokenInfo{UserID: strconv.FormatInt(o.UserID, 10), Expiration: o.Expires,
				Scopes: []string{auth.OAuthScope}, Extra: map[string]any{"owner": o}}, nil
		},
		ResourceMetadataURL: site + "/.well-known/oauth-protected-resource/mcp",
		OnToolCall: func(ctx context.Context, userID, tool string) {
			id, err := strconv.ParseInt(userID, 10, 64)
			if err != nil {
				return
			}
			if err := store.CountMCPCall(context.WithoutCancel(ctx), id, tool); err != nil {
				slog.Error("count mcp call", "error", err)
			}
		},
	})
	wrapped := gin.WrapH(h)
	perIP := newLimiter(600, 300).middleware()
	r.Any("/mcp", perIP, wrapped)
	r.Any("/mcp/sse", perIP, wrapped)
}
