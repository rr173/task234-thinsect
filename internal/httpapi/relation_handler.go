package httpapi

import (
	"net/http"

	"task234-thinsect/internal/relation"
)

func (a *API) detectRelations(w http.ResponseWriter, r *http.Request) {
	batchID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := a.app.Review.GuardFrozen(batchID); err != nil {
		writeErr(w, err)
		return
	}
	res, err := a.app.Relation.Detect(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) listRelations(w http.ResponseWriter, r *http.Request) {
	batchID, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	list, err := a.app.Relation.ListByBatch(batchID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) adjudicateRelation(w http.ResponseWriter, r *http.Request) {
	id, ok := idOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		Confirmed bool   `json:"confirmed"`
		Note      string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.guardFrozenRelation(id); err != nil {
		writeErr(w, err)
		return
	}
	rel, err := a.app.Relation.Adjudicate(relation.AdjudicateInput{
		RelationID: id,
		Confirmed:  req.Confirmed,
		Note:       req.Note,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// guardFrozenRelation 冻结版本存在时拒绝关系写操作。
func (a *API) guardFrozenRelation(relID int64) error {
	rel, err := a.app.Rels.Get(relID)
	if err != nil {
		return err
	}
	return a.app.Review.GuardFrozen(rel.BatchID)
}
