// Package httpapi 提供薄片复核台的 REST API（统一 /api 前缀）。
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"task234-thinsect/internal/model"
	"task234-thinsect/internal/service"
)

// API 持有服务门禁并注册路由。
type API struct {
	app *service.App
}

// New 创建 HTTP 处理器并注册全部路由。
func New(app *service.App) http.Handler {
	a := &API{app: app}
	mux := http.NewServeMux()

	// 批次
	mux.HandleFunc("POST /api/batches", a.createBatch)
	mux.HandleFunc("GET /api/batches", a.listBatches)
	mux.HandleFunc("GET /api/batches/{id}", a.getBatch)
	mux.HandleFunc("POST /api/batches/{id}/advance", a.advanceBatch)
	mux.HandleFunc("GET /api/batches/{id}/stats", a.batchStats)

	// 图像摘要
	mux.HandleFunc("POST /api/batches/{id}/images", a.importImage)
	mux.HandleFunc("GET /api/batches/{id}/images", a.listImages)
	mux.HandleFunc("GET /api/images/{id}", a.getImage)

	// 区域
	mux.HandleFunc("POST /api/images/{id}/regions", a.importRegion)
	mux.HandleFunc("GET /api/images/{id}/regions", a.listImageRegions)
	mux.HandleFunc("GET /api/regions/{id}", a.getRegion)
	mux.HandleFunc("POST /api/regions/{id}/label", a.labelRegion)
	mux.HandleFunc("POST /api/regions/{id}/mismerged", a.markMismerged)
	mux.HandleFunc("POST /api/regions/{id}/open-boundary", a.markOpenBoundary)
	mux.HandleFunc("POST /api/regions/{id}/exclude", a.excludeRegion)
	mux.HandleFunc("POST /api/regions/{id}/split", a.splitRegion)

	// 特征
	mux.HandleFunc("POST /api/regions/{id}/features", a.computeFeatures)
	mux.HandleFunc("GET /api/regions/{id}/features", a.getFeatures)

	// 关系
	mux.HandleFunc("POST /api/batches/{id}/relations/detect", a.detectRelations)
	mux.HandleFunc("GET /api/batches/{id}/relations", a.listRelations)
	mux.HandleFunc("POST /api/relations/{id}/adjudicate", a.adjudicateRelation)

	// 意见
	mux.HandleFunc("POST /api/regions/{id}/opinions", a.addOpinion)
	mux.HandleFunc("GET /api/regions/{id}/opinions", a.listOpinions)

	// 版本
	mux.HandleFunc("POST /api/batches/{id}/versions", a.createVersion)
	mux.HandleFunc("GET /api/batches/{id}/versions", a.listVersions)
	mux.HandleFunc("POST /api/versions/{id}/share", a.shareVersion)
	mux.HandleFunc("POST /api/versions/{id}/freeze", a.freezeVersion)
	mux.HandleFunc("POST /api/versions/{id}/supersede", a.supersedeVersion)

	// 基础
	mux.HandleFunc("GET /api/minerals", a.listMinerals)
	mux.HandleFunc("GET /api/health", a.health)
	return mux
}

// idOf 解析路径中的数字 ID。
func idOf(r *http.Request) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return v, err == nil
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 按领域错误映射 HTTP 状态码。
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrFrozenVersion):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, model.ErrBadState), errors.As(err, new(*model.StateTransitionError)):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, model.ErrValidation), errors.Is(err, model.ErrInvalidGeometry),
		errors.Is(err, model.ErrRegionOutOfBounds), errors.Is(err, model.ErrSelfIntersecting),
		errors.Is(err, model.ErrNotClosed), errors.Is(err, model.ErrUnknownMineral),
		errors.Is(err, model.ErrImageModeMismatch), errors.Is(err, model.ErrClosedLoopRequired):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

// decode 解析请求体到目标结构。
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return false
	}
	return true
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "thinsect"})
}
