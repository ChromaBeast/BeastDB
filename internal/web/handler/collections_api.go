package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// CollectionStore is the engine surface the collections endpoint needs.
// *api.Engine satisfies it; engines without it get 501.
type CollectionStore interface {
	ListCollections() ([]api.Collection, error)
	CreateCollection(name string, prefix *uint8) (api.Collection, error)
	DropCollection(name string) error
}

// CollectionEntry is the JSON shape of one collection.
type CollectionEntry struct {
	Name      string `json:"name"`
	Prefix    uint8  `json:"prefix"`
	CreatedAt int64  `json:"createdAt"`
}

func toEntry(c api.Collection) CollectionEntry {
	return CollectionEntry{Name: c.Name, Prefix: c.Prefix, CreatedAt: c.CreatedAt}
}

// CollectionsHandler serves /api/collections: GET lists (any signed-in user),
// POST {name, prefix?} creates and DELETE ?name= drops (admin only).
func (h *APIHandler) CollectionsHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := h.engine.(CollectionStore)
	if !ok {
		http.Error(w, "Collections are not supported by this engine", http.StatusNotImplemented)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cols, err := store.ListCollections()
		if err != nil {
			http.Error(w, "Engine error", http.StatusInternalServerError)
			return
		}
		out := make([]CollectionEntry, len(cols))
		for i, c := range cols {
			out[i] = toEntry(c)
		}
		writeJSON(w, out)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var body struct {
			Name   string `json:"name"`
			Prefix *int   `json:"prefix"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		var prefix *uint8
		if body.Prefix != nil {
			if *body.Prefix < 0 || *body.Prefix > 255 {
				http.Error(w, "prefix must be 0-255", http.StatusBadRequest)
				return
			}
			p := uint8(*body.Prefix)
			prefix = &p
		}
		col, err := store.CreateCollection(body.Name, prefix)
		if err != nil {
			writeCollectionError(w, err)
			return
		}
		h.record(r, audit.ActionCollectionCreate, col.Name, "prefix "+strconv.Itoa(int(col.Prefix)))
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, toEntry(col))
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		name := r.URL.Query().Get("name")
		if err := store.DropCollection(name); err != nil {
			writeCollectionError(w, err)
			return
		}
		h.record(r, audit.ActionCollectionDrop, name, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func writeCollectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, api.ErrInvalidCollectionName), errors.Is(err, api.ErrReservedPrefix):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, api.ErrCollectionExists), errors.Is(err, api.ErrCollectionNotEmpty), errors.Is(err, api.ErrConditionFailed):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, api.ErrCollectionNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, api.ErrNoFreePrefix):
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
	case errors.Is(err, api.ErrReadOnlyReplica):
		http.Error(w, "This node is a read-only replica", http.StatusConflict)
	default:
		http.Error(w, "Engine error", http.StatusInternalServerError)
	}
}
