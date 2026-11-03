package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// iconifyBase is the icon repository's API root. It is a variable so the
// handler tests can point it at a fake server instead of the internet.
var iconifyBase = "https://api.iconify.design"

// An Iconify id is "<prefix>:<name>", both lowercase slugs. Validating the
// shape before echoing it keeps arbitrary text out of the response; the URL
// itself is always built against iconifyBase, so there is no SSRF surface.
var iconIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}:[a-z0-9][a-z0-9-]{0,63}$`)

const (
	iconSearchTTL   = time.Hour
	iconSearchMax   = 200
	iconSearchLimit = 24
)

// Colour first: the logos/flat-colour/twemoji sets carry their own palette,
// while MDI and Simple Icons are monochrome and the client tints them. Ranking
// by set keeps "netflix" showing the logo before the glyph, and "tv" showing a
// colourful television before the outline.
// Iconify matches names, not meanings: "tv" does not find twemoji:television.
// A few everyday synonyms bridge that gap so the colourful generic icons are
// reachable by the word an operator actually types.
var iconSearchSynonyms = map[string][]string{
	"tv":     {"television"},
	"telly":  {"television"},
	"movie":  {"film"},
	"movies": {"film"},
}

var iconPrefixRank = map[string]int{
	"logos":            0,
	"flat-color-icons": 1,
	"twemoji":          2,
	"mdi":              3,
	"simple-icons":     4,
}

type iconSearchEntry struct {
	icons []string
	at    time.Time
}

var iconSearchCache = struct {
	sync.Mutex
	entries map[string]iconSearchEntry
}{entries: map[string]iconSearchEntry{}}

// handleIconSearch proxies the icon repository's search so the dashboard can
// offer suggestions without widening connect-src to the third party. The
// catalogue is limited to MDI (generic TV, music, sport…) and Simple Icons
// (brand glyphs); the client turns each id into an SVG URL and stores that URL
// in service_icons, so rendering needs no proxy of its own.
func (ws *WebServer) handleIconSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 || len(q) > 64 {
		writeIconSearch(w, []string{})
		return
	}

	if icons, ok := cachedIconSearch(q); ok {
		writeIconSearch(w, icons)
		return
	}

	icons, err := fetchIconSearch(r.Context(), q)
	if err != nil {
		http.Error(w, "icon search unavailable", http.StatusBadGateway)
		return
	}
	storeIconSearch(q, icons)
	writeIconSearch(w, icons)
}

func writeIconSearch(w http.ResponseWriter, icons []string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_ = json.NewEncoder(w).Encode(map[string]any{"icons": icons})
}

func fetchIconSearch(parent context.Context, q string) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()

	terms := append([]string{strings.ToLower(q)}, iconSearchSynonyms[strings.ToLower(q)]...)
	seen := map[string]bool{}
	out := make([]string, 0, iconSearchLimit)
	var lastErr error
	succeeded := false

	for _, term := range terms {
		ids, err := requestIconSearch(ctx, term)
		if err != nil {
			lastErr = err
			continue
		}
		succeeded = true
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	if !succeeded {
		if lastErr == nil {
			lastErr = context.DeadlineExceeded
		}
		return nil, lastErr
	}

	sort.SliceStable(out, func(i, j int) bool {
		return iconRank(out[i]) < iconRank(out[j])
	})
	if len(out) > iconSearchLimit {
		out = out[:iconSearchLimit]
	}
	return out, nil
}

// requestIconSearch asks the repository for one term and answers only ids that
// match the shape an Iconify id has; arbitrary upstream text is dropped.
func requestIconSearch(ctx context.Context, term string) ([]string, error) {
	endpoint := iconifyBase + "/search?query=" + url.QueryEscape(term) +
		"&limit=" + strconv.Itoa(iconSearchLimit*2) +
		"&prefixes=logos,flat-color-icons,twemoji,mdi,simple-icons"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := (&http.Client{Timeout: 7 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("icon search returned HTTP %d", resp.StatusCode)
	}

	var body struct {
		Icons []string `json:"icons"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(body.Icons))
	for _, id := range body.Icons {
		if iconIDPattern.MatchString(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func iconRank(id string) int {
	prefix, _, ok := strings.Cut(id, ":")
	if !ok {
		return 99
	}
	if rank, ok := iconPrefixRank[prefix]; ok {
		return rank
	}
	return 99
}

func cachedIconSearch(q string) ([]string, bool) {
	iconSearchCache.Lock()
	defer iconSearchCache.Unlock()
	entry, ok := iconSearchCache.entries[q]
	if !ok || time.Since(entry.at) > iconSearchTTL {
		if ok {
			delete(iconSearchCache.entries, q)
		}
		return nil, false
	}
	return entry.icons, true
}

func storeIconSearch(q string, icons []string) {
	iconSearchCache.Lock()
	defer iconSearchCache.Unlock()
	if len(iconSearchCache.entries) >= iconSearchMax {
		// Bounded cache: a fresh catalogue is cheap to refetch, so clear rather
		// than carry an LRU list for a query set that is tiny in practice.
		iconSearchCache.entries = map[string]iconSearchEntry{}
	}
	iconSearchCache.entries[q] = iconSearchEntry{icons: icons, at: time.Now()}
}

func clearIconSearchCache() {
	iconSearchCache.Lock()
	defer iconSearchCache.Unlock()
	iconSearchCache.entries = map[string]iconSearchEntry{}
}
