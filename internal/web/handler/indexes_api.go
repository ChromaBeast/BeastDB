package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/index"
)

// IndexStore is the engine surface the indexes endpoint needs (ADR 0006).
// *api.Engine satisfies it; engines without it get 501.
type IndexStore interface {
	ListIndexes() ([]api.IndexInfo, error)
	CreateIndex(name, collection, field string, kind int) (api.IndexDef, error)
	DropIndex(name string) error
	ScanIndexed(filter api.IndexFilter, startKey, endKey uint64, limit int) ([]api.KeyValue, bool, error)
}

// IndexEntry is the JSON shape of one index, exactly what the server reports.
type IndexEntry struct {
	Name       string `json:"name"`
	Collection string `json:"collection"`
	Prefix     uint8  `json:"prefix"`
	Field      string `json:"field"`
	Type       string `json:"type"` // "int" or "string"
	State      string `json:"state"`
	Entries    int    `json:"entries"`
	Skipped    int    `json:"skipped"`
	Examined   int64  `json:"examined"`
	CreatedAt  int64  `json:"createdAt"`
}

func kindName(kind int) string {
	if kind == index.KindString {
		return "string"
	}
	return "int"
}

func toIndexEntry(i api.IndexInfo) IndexEntry {
	return IndexEntry{Name: i.Name, Collection: i.Collection, Prefix: i.Prefix, Field: i.Field, Type: kindName(i.Kind),
		State: i.State, Entries: i.Entries, Skipped: i.Skipped, Examined: i.Examined, CreatedAt: i.CreatedAt}
}

// IndexesHandler serves /api/indexes: GET lists (any signed-in user),
// POST {name, collection, field, type} creates and DELETE ?name= drops (admin only).
func (h *APIHandler) IndexesHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := h.raw.(IndexStore)
	if !ok {
		http.Error(w, "Indexes are not supported by this engine", http.StatusNotImplemented)
		return
	}
	switch r.Method {
	case http.MethodGet:
		infos, err := store.ListIndexes()
		if err != nil {
			http.Error(w, "Engine error", http.StatusInternalServerError)
			return
		}
		out := make([]IndexEntry, len(infos))
		for i, x := range infos {
			out[i] = toIndexEntry(x)
		}
		writeJSON(w, out)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var body struct {
			Name       string `json:"name"`
			Collection string `json:"collection"`
			Field      string `json:"field"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		kind := 0
		switch body.Type {
		case "int":
			kind = index.KindInt
		case "string":
			kind = index.KindString
		default:
			http.Error(w, `type must be "int" or "string"`, http.StatusBadRequest)
			return
		}
		def, err := store.CreateIndex(body.Name, body.Collection, body.Field, kind)
		if err != nil {
			writeIndexError(w, err)
			return
		}
		h.record(r, audit.ActionIndexCreate, def.Name, body.Collection+"."+def.Field+" "+body.Type)
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"name": def.Name})
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		name := r.URL.Query().Get("name")
		if err := store.DropIndex(name); err != nil {
			writeIndexError(w, err)
			return
		}
		h.record(r, audit.ActionIndexDrop, name, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func writeIndexError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, api.ErrInvalidIndex), errors.Is(err, api.ErrInvalidCollectionName), errors.Is(err, api.ErrIndexFilter):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, api.ErrIndexExists), errors.Is(err, api.ErrConditionFailed), errors.Is(err, api.ErrIndexBuilding):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, api.ErrIndexNotFound), errors.Is(err, api.ErrCollectionNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, api.ErrTooManyIndexes):
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
	case errors.Is(err, api.ErrReadOnlyReplica):
		http.Error(w, "This node is a read-only replica", http.StatusConflict)
	default:
		http.Error(w, "Engine error", http.StatusInternalServerError)
	}
}

// getRecordsByIndex serves GET /api/records?index=NAME&eq=V (or gte=/lte=):
// matching records in ascending key order, paged like the unfiltered scan.
func (h *APIHandler) getRecordsByIndex(w http.ResponseWriter, r *http.Request, start, end uint64, limit int) {
	store, ok := h.raw.(IndexStore)
	if !ok {
		http.Error(w, "Indexes are not supported by this engine", http.StatusNotImplemented)
		return
	}
	q := r.URL.Query()
	infos, err := store.ListIndexes()
	if err != nil {
		http.Error(w, "Engine error", http.StatusInternalServerError)
		return
	}
	kind := 0
	for _, i := range infos {
		if i.Name == q.Get("index") {
			kind = i.Kind
		}
	}
	if kind == 0 {
		http.Error(w, api.ErrIndexNotFound.Error(), http.StatusNotFound)
		return
	}
	parse := func(name string) (*api.IndexValue, bool) {
		if !q.Has(name) {
			return nil, true
		}
		raw := q.Get(name)
		if kind == index.KindString {
			return &api.IndexValue{IsString: true, Str: raw}, true
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, false
		}
		return &api.IndexValue{Int: n}, true
	}
	var f api.IndexFilter
	f.Index = q.Get("index")
	var okEq, okGte, okLte bool
	f.Eq, okEq = parse("eq")
	f.Gte, okGte = parse("gte")
	f.Lte, okLte = parse("lte")
	if !okEq || !okGte || !okLte {
		http.Error(w, "filter value does not match the index type", http.StatusBadRequest)
		return
	}
	if limit > 500 {
		limit = 500
	}
	items, more, err := store.ScanIndexed(f, start, end, limit)
	if err != nil {
		writeIndexError(w, err)
		return
	}
	records := make([]api.RecordItem, len(items))
	for i, it := range items {
		records[i] = api.RecordItem{Key: it.Key, KeyText: strconv.FormatUint(it.Key, 10), Value: string(it.Value)}
	}
	var next uint64
	more = more && len(items) > 0 // no record to resume from: stop rather than loop
	if more {
		next = items[len(items)-1].Key + 1
	}
	writeJSON(w, map[string]any{
		"records":     records,
		"count":       len(records),
		"nextKey":     next,
		"nextKeyText": strconv.FormatUint(next, 10),
		"hasMore":     more,
	})
}
