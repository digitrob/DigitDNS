package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleV3Filtering exposes the dashboard's filtering status on the v3 surface.
// The body is the same document the panel reads, including settings, installed
// lists, the preset catalogue and the cached preset domain counts.
func (a *API) handleV3Filtering(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	resp, err := a.v3Backend.FilteringStatus(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "filtering_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleV3FilteringSettings saves the policy that applies to every list. POST is
// the panel's verb; PUT is accepted because the v3 surface is otherwise a full
// replace.
func (a *API) handleV3FilteringSettings(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST or PUT only")
		return
	}
	resp, err := a.v3Backend.SaveFilteringSettings(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleV3FilteringRefresh starts a background refresh of every enabled list.
func (a *API) handleV3FilteringRefresh(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	resp, err := a.v3Backend.RefreshFiltering(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "filtering_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleV3FilteringPresets reads the curated catalogue or installs a batch of
// preset IDs. Installation is asynchronous: the response reports which IDs were
// added, skipped as duplicates, or rejected.
func (a *API) handleV3FilteringPresets(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST only")
		return
	}
	resp, err := a.v3Backend.FilteringPresets(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleV3FilteringLists adds a URL subscription or an uploaded list body. The
// request body is passed through untouched: the dashboard handler owns validation
// and the 20 MB cap, so a large upload is not rejected by the v3 router first.
func (a *API) handleV3FilteringLists(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	resp, err := a.v3Backend.AddFilteringList(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleV3FilteringListItem owns two shapes: /lists/{id} for enable, disable and
// delete, and /lists/{id}/update for a one-list refresh.
func (a *API) handleV3FilteringListItem(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v3/filtering/lists/"), "/")
	if rest == "" {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "missing filter list id")
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) > 2 || (len(parts) == 2 && parts[1] != "update") {
		writeProblemAt(w, r, http.StatusNotFound, "not_found", "unknown filter list action")
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
			return
		}
	} else if r.Method != http.MethodPatch && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "PATCH, POST or DELETE")
		return
	}
	resp, err := a.v3Backend.FilteringListItem(r, rest)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
