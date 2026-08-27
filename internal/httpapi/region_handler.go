package httpapi

import (
	"net/http"

	"task234-thinsect/internal/feature"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
)

func (a *API) importRegion(w http.ResponseWriter, r *http.Request) {
	imageID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		BatchID     int64         `json:"batch_id"`
		Label       string        `json:"label"`
		MineralCode string        `json:"mineral_code"`
		Vertices    []model.Point `json:"vertices"`
	}
	if !decode(w, r, &req) {
		return
	}
	region, err := a.app.Segment.Import(segment.RegionInput{
		BatchID:     req.BatchID,
		ImageID:     imageID,
		Label:       req.Label,
		MineralCode: req.MineralCode,
		Polygon:     model.Polygon{Vertices: req.Vertices},
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, region)
}

func (a *API) listImageRegions(w http.ResponseWriter, r *http.Request) {
	imageID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	list, err := a.app.Segment.ListByImage(imageID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) getRegion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	region, err := a.app.Segment.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, region)
}

func (a *API) labelRegion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		MineralCode string `json:"mineral_code"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.guardFrozenRegion(id); err != nil {
		writeErr(w, err)
		return
	}
	region, err := a.app.Segment.Label(id, req.MineralCode)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, region)
}

func (a *API) markMismerged(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := a.guardFrozenRegion(id); err != nil {
		writeErr(w, err)
		return
	}
	region, err := a.app.Segment.MarkMismerged(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, region)
}

func (a *API) markOpenBoundary(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := a.guardFrozenRegion(id); err != nil {
		writeErr(w, err)
		return
	}
	region, err := a.app.Segment.MarkOpenBoundary(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, region)
}

func (a *API) excludeRegion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := a.guardFrozenRegion(id); err != nil {
		writeErr(w, err)
		return
	}
	region, err := a.app.Segment.Exclude(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, region)
}

func (a *API) splitRegion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := a.guardFrozenRegion(id); err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		Parts [][]model.Point `json:"parts"`
	}
	if !decode(w, r, &req) {
		return
	}
	parts := make([]model.Polygon, 0, len(req.Parts))
	for _, verts := range req.Parts {
		parts = append(parts, model.Polygon{Vertices: verts})
	}
	created, err := a.app.Segment.Split(segment.SplitInput{RegionID: id, Parts: parts})
	if err != nil {
		writeErr(w, err)
		return
	}
	for i := range created {
		created[i].ParentRegionID = nil
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) computeFeatures(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	f, err := a.app.Feature.Compute(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *API) getFeatures(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	region, err := a.app.Segment.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	// 展示已持久化的特征（未计算过则为零值）。
	f := feature.Feature{
		RegionID:        region.ID,
		AvgR:            region.AvgR,
		AvgG:            region.AvgG,
		AvgB:            region.AvgB,
		ExtinctionRatio: region.ExtinctionRatio,
		ExtAngle:        region.ExtAngle,
	}
	if region.MineralCode != "" {
		if m, err := a.app.Minerals.Get(region.MineralCode); err == nil {
			f.CandidateCode = m.Code
			f.CandidateName = m.Name
		}
	}
	writeJSON(w, http.StatusOK, f)
}

// guardFrozenRegion 冻结版本存在时拒绝区域写操作。
func (a *API) guardFrozenRegion(regionID int64) error {
	region, err := a.app.Segment.Get(regionID)
	if err != nil {
		return err
	}
	return a.app.Review.GuardFrozen(region.BatchID)
}
