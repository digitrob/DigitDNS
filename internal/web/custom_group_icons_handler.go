package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"digitdns/internal/httpx"
)

// Custom-group icon endpoints (v2.7): GET serves the stored icon, PUT replaces
// it, DELETE removes it. All under the authenticated /api/custom-groups/ tree.
// The upload standard (sizes, the SVG allowlist re-encode, the PNG ceiling) is
// documented in custom_group_icons.go.
func (ws *WebServer) handleCustomGroupIcon(w http.ResponseWriter, r *http.Request, groupID string) {
	switch r.Method {
	case http.MethodGet:
		icon, ok := ws.db.GetCustomGroupIcon(groupID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", groupIconContentType(icon.Kind))
		w.Header().Set("Cache-Control", "private, max-age=60")
		w.Header().Set("ETag", groupIconETag(icon))
		w.Header().Set("Content-Length", strconv.Itoa(len(icon.Data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(icon.Data)
		}
	case http.MethodPut, http.MethodPost:
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGroupIconUploadByte))
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "the icon exceeds the 64 KiB upload limit")
			return
		}
		if _, ok := ws.customGroups.Get(groupID); !ok {
			httpx.WriteJSONError(w, http.StatusNotFound, "no custom group with that id")
			return
		}
		icon, err := processGroupIconUpload(raw)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ws.db.SaveCustomGroupIcon(groupID, icon); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "could not store the icon")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "kind": icon.Kind, "bytes": len(icon.Data)})
	case http.MethodDelete:
		if err := ws.db.DeleteCustomGroupIcon(groupID); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "could not remove the icon")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, POST, PUT, DELETE")
	}
}

// parseCustomGroupPath splits /api/custom-groups/{id}[/icon] into its parts.
// The second segment is optional and must be exactly "icon".
func parseCustomGroupPath(p string) (id, sub string, ok bool) {
	rest := strings.TrimPrefix(p, "/api/custom-groups/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return "", "", false
	}
	segments := strings.Split(rest, "/")
	switch len(segments) {
	case 1:
		return segments[0], "", true
	case 2:
		if segments[1] == "icon" {
			return segments[0], segments[1], true
		}
		return "", "", false
	default:
		return "", "", false
	}
}
