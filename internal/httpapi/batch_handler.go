package httpapi

import (
	"net/http"

	"task234-thinsect/internal/image"
)

func (a *API) createBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		RockType string `json:"rock_type"`
		Locality string `json:"locality"`
	}
	if !decode(w, r, &req) {
		return
	}
	b, err := a.app.CreateBatch(req.Code, req.RockType, req.Locality)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (a *API) listBatches(w http.ResponseWriter, r *http.Request) {
	list, err := a.app.ListBatches()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) getBatch(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	b, err := a.app.GetBatch(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (a *API) advanceBatch(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if !decode(w, r, &req) {
		return
	}
	b, err := a.app.AdvanceBatch(id, req.To)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (a *API) batchStats(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	s, err := a.app.Stats(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) importImage(w http.ResponseWriter, r *http.Request) {
	batchID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req image.ImportInput
	if !decode(w, r, &req) {
		return
	}
	req.BatchID = batchID
	img, deduped, err := a.app.Image.Import(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusCreated
	if deduped {
		status = http.StatusOK // 幂等：返回既有记录
	}
	writeJSON(w, status, img)
}

func (a *API) listImages(w http.ResponseWriter, r *http.Request) {
	batchID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	list, err := a.app.Image.ListByBatch(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) getImage(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	img, err := a.app.Image.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, img)
}

func (a *API) listMinerals(w http.ResponseWriter, r *http.Request) {
	list, err := a.app.Minerals.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
