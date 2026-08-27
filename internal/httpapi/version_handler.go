package httpapi

import "net/http"

func (a *API) addOpinion(w http.ResponseWriter, r *http.Request) {
	regionID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
		Author  string `json:"author"`
	}
	if !decode(w, r, &req) {
		return
	}
	o, err := a.app.Review.AddOpinion(regionID, req.Kind, req.Content, req.Author)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (a *API) listOpinions(w http.ResponseWriter, r *http.Request) {
	regionID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	list, err := a.app.Review.ListOpinions(regionID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) createVersion(w http.ResponseWriter, r *http.Request) {
	batchID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		Name    string `json:"name"`
		Summary string `json:"summary"`
	}
	if !decode(w, r, &req) {
		return
	}
	v, err := a.app.Review.CreateVersion(batchID, req.Name, req.Summary)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (a *API) listVersions(w http.ResponseWriter, r *http.Request) {
	batchID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	list, err := a.app.Review.ListVersions(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) shareVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	v, err := a.app.Review.ShareVersion(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) freezeVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	v, err := a.app.Review.FreezeVersion(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) supersedeVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	v, err := a.app.Review.SupersedeVersion(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
