package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/component"
)

const specLimit = 256 << 10

func bindSpecJSON(c *gin.Context, v any) bool {
	dec := json.NewDecoder(io.LimitReader(c.Request.Body, specLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "內容格式錯誤："+err.Error())
		return false
	}
	return true
}

func componentFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, component.ErrInvalid):
		msg := strings.TrimPrefix(err.Error(), component.ErrInvalid.Error()+": ")
		writeError(c, http.StatusUnprocessableEntity, "invalid_component", "檢查沒有通過："+msg)
	case errors.Is(err, component.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "找不到這個元件或版本")
	case errors.Is(err, component.ErrExists):
		writeError(c, http.StatusConflict, "exists", "這個代碼已經有元件使用")
	case errors.Is(err, component.ErrConflict):
		writeError(c, http.StatusConflict, "conflict", "這個元件在你開啟後被修改過，請重新載入再編輯")
	default:
		adminFail(c, err)
	}
}

func (h adminHandlers) reader() (component.QueryFunc, bool) {
	return h.query, h.query != nil
}

func (h adminHandlers) components(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	list, err := component.List(c.Request.Context(), h.db)
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, list, nil)
}

func (h adminHandlers) componentSpec(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	spec, version, err := component.Latest(c.Request.Context(), h.db, c.Param("slug"))
	if err != nil {
		componentFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"spec": spec, "version": version}, nil)
}

type saveInput struct {
	Spec        component.ComponentSpec `json:"spec"`
	Note        string                  `json:"note"`
	BaseVersion *int64                  `json:"base_version"`
}

func (h adminHandlers) createComponent(c *gin.Context) {
	var in saveInput
	h.save(c, &in, true)
}

func (h adminHandlers) updateComponent(c *gin.Context) {
	var in saveInput
	h.save(c, &in, false)
}

func (h adminHandlers) save(c *gin.Context, in *saveInput, create bool) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	run, ok := h.reader()
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "這個伺服器沒有設定唯讀查詢帳號")
		return
	}
	if !bindSpecJSON(c, in) {
		return
	}
	if !create && in.Spec.Slug != c.Param("slug") {
		writeError(c, http.StatusBadRequest, "invalid_slug", "元件代碼不能修改")
		return
	}
	if !create && in.BaseVersion == nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "缺少 base_version")
		return
	}
	note := truncateRunes(in.Note, 200)
	if note == "" {
		note = map[bool]string{true: "建立", false: "後台編輯"}[create]
	}
	ctx := c.Request.Context()
	version, err := component.Save(ctx, h.db, run, in.Spec, component.SaveOptions{
		Note: note, By: &u.ID, Create: create, BaseVersion: in.BaseVersion,
	})
	if err != nil {
		componentFail(c, err)
		return
	}
	action := map[bool]string{true: "component.create", false: "component.update"}[create]
	h.audit(ctx, u.ID, c, action, "component:"+in.Spec.Slug, map[string]any{"version": version, "note": note})
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	writeData(c, status, gin.H{"version": version}, nil)
}

func (h adminHandlers) previewComponent(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	run, ok := h.reader()
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "這個伺服器沒有設定唯讀查詢帳號")
		return
	}
	var in struct {
		Spec   component.ComponentSpec `json:"spec"`
		County string                  `json:"county"`
	}
	if !bindSpecJSON(c, &in) {
		return
	}
	ctx := c.Request.Context()
	if in.County == "" {
		in.County = component.SampleCounty
	} else if ok, err := h.countyExists(ctx, in.County); err != nil {
		adminFail(c, err)
		return
	} else if !ok {
		writeError(c, http.StatusBadRequest, "invalid_county", "縣市代碼不存在")
		return
	}
	checks, err := component.Preview(ctx, h.db, run, in.Spec, in.County)
	if err != nil {
		componentFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"county": in.County, "checks": checks}, nil)
}

func (h adminHandlers) componentVersions(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	list, err := component.Versions(c.Request.Context(), h.db, c.Param("slug"))
	if err != nil {
		componentFail(c, err)
		return
	}
	writeData(c, http.StatusOK, list, nil)
}

func versionParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("version"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個版本")
		return 0, false
	}
	return id, true
}

func (h adminHandlers) componentVersion(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	id, ok := versionParam(c)
	if !ok {
		return
	}
	spec, err := component.VersionSpec(c.Request.Context(), h.db, c.Param("slug"), id)
	if err != nil {
		componentFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"spec": spec, "version": id}, nil)
}

func (h adminHandlers) restoreVersion(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	run, ok := h.reader()
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "這個伺服器沒有設定唯讀查詢帳號")
		return
	}
	id, ok := versionParam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	slug := c.Param("slug")
	spec, err := component.VersionSpec(ctx, h.db, slug, id)
	if err != nil {
		componentFail(c, err)
		return
	}
	note := fmt.Sprintf("還原版本 #%d", id)
	version, err := component.Save(ctx, h.db, run, spec, component.SaveOptions{Note: note, By: &u.ID})
	if err != nil {
		componentFail(c, err)
		return
	}
	h.audit(ctx, u.ID, c, "component.restore", "component:"+slug, map[string]any{"from": id, "version": version})
	writeData(c, http.StatusOK, gin.H{"version": version}, nil)
}

func (h adminHandlers) setStatus(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	run, ok := h.reader()
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "這個伺服器沒有設定唯讀查詢帳號")
		return
	}
	var in struct {
		Status      string `json:"status"`
		BaseVersion int64  `json:"base_version"`
	}
	if !bindJSON(c, &in) {
		return
	}
	if in.Status != "draft" && in.Status != "published" {
		writeError(c, http.StatusBadRequest, "invalid_status", "狀態只能是 draft 或 published")
		return
	}
	ctx := c.Request.Context()
	slug := c.Param("slug")
	spec, latest, err := component.Latest(ctx, h.db, slug)
	if err != nil {
		componentFail(c, err)
		return
	}
	if latest != in.BaseVersion {
		componentFail(c, component.ErrConflict)
		return
	}
	if spec.Status == in.Status {
		writeData(c, http.StatusOK, gin.H{"version": latest}, nil)
		return
	}
	spec.Status = in.Status
	note := map[string]string{"published": "發布", "draft": "取消發布"}[in.Status]
	version, err := component.Save(ctx, h.db, run, spec, component.SaveOptions{Note: note, By: &u.ID, BaseVersion: &latest})
	if err != nil {
		componentFail(c, err)
		return
	}
	h.audit(ctx, u.ID, c, "component."+map[string]string{"published": "publish", "draft": "unpublish"}[in.Status],
		"component:"+slug, map[string]any{"version": version})
	writeData(c, http.StatusOK, gin.H{"version": version}, nil)
}

func (h adminHandlers) audit(ctx context.Context, userID int64, c *gin.Context, action, target string, detail map[string]any) {
	if err := h.store.Audit(context.WithoutCancel(ctx), &userID, action, target, detail, c.ClientIP()); err != nil {
		slog.Error("audit", "action", action, "error", err)
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}
