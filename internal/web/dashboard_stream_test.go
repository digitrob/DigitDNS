package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/database"
)

// The live query console had three variants of one bug: a control that reads its own value at
// the wrong moment does nothing observable, and nothing anywhere reports it.
//
// The filter dropdown and the search box had no event listeners at all. Both were read inside
// the function that appended an arriving row, so they applied to queries that had not happened
// yet and never to the eighty rows already on screen. Selecting BLOCK left every DIRECT row in
// place; typing a domain into the search box changed nothing until the next query came in,
// which on a resolver serving one household is minutes. The control looked broken because,
// from the operator's side, it was.
//
// The same append path cleared the "Listening for live DNS queries..." placeholder *before* it
// applied the filter, so a query that was then filtered out took the placeholder with it: the
// table went blank, with no row of any kind, and stayed blank.
//
// And Clear emptied the table's markup directly. The placeholder was recognised by matching
// the string "Listening" against the first row's text, so once the markup was gone it could
// never come back — a cleared stream and a dead daemon looked exactly alike.
//
// All three are fixed by holding the queries as data (streamBuffer) and rendering the table
// from it. These tests pin the parts of that arrangement which can be checked without a
// JavaScript runtime: that the two controls re-render, that Clear empties the buffer rather
// than the markup, that the placeholder is identified by a class both files agree on, and that
// the SSE handlers feed the buffer instead of writing to the table themselves.

// streamToolbarFilters are the two controls whose entire purpose is to change what is already
// on screen. A control like this is not wired merely by being read somewhere.
var streamToolbarFilters = []string{"stream-filter", "stream-search"}

func TestStreamFilterControlsRerenderExistingRows(t *testing.T) {
	body := functionBody(t, readAsset(t, "js/app.js"), "initEventListeners")

	for _, id := range streamToolbarFilters {
		wired := false
		for line := range strings.SplitSeq(body, "\n") {
			if !strings.Contains(line, "'"+id+"'") {
				continue
			}
			if strings.Contains(line, "addEventListener") && strings.Contains(line, "renderQueryStream") {
				wired = true
				break
			}
		}
		if !wired {
			t.Errorf("#%s has no listener in initEventListeners that calls renderQueryStream.\n"+
				"Without one it only affects rows that arrive after it changes: the queries "+
				"already in the table are never re-filtered, so on a quiet resolver the control "+
				"appears to do nothing at all.", id)
		}
	}
}

// Clear has to empty the data, not the markup — and it has to empty the daemon's ring as
// well. A client-only clear was undone by the next SSE reconnect, which replayed the
// history the operator had just dismissed; and a coalesced insert frame still holding
// evicted arrivals could repaint a handful of rows after the buffer was reset.
func TestStreamClearEmptiesBufferPendingWorkAndTheServerRing(t *testing.T) {
	app := readAsset(t, "js/app.js")
	body := functionBody(t, app, "initEventListeners")

	if !strings.Contains(body, "'stream-clear-btn'") {
		t.Fatal("initEventListeners no longer binds #stream-clear-btn — the button does nothing")
	}
	if !strings.Contains(body, "clearQueryStream()") {
		t.Error("the #stream-clear-btn handler does not call clearQueryStream(), so Clear does " +
			"not reset the view or the server ring.")
	}

	reset := functionBody(t, app, "resetStreamView")
	for _, want := range []string{
		"streamBuffer = []",
		"streamRendered = []",
		"streamPendingQueries = []",
		"streamPendingDropped = []",
		"streamPendingRender = false",
		"streamEpoch++",
		"renderQueryStream()",
	} {
		if !strings.Contains(reset, want) {
			t.Errorf("resetStreamView does not %q; a Clear would leave state that repaints rows", want)
		}
	}

	clearFn := functionBody(t, app, "clearQueryStream")
	if !strings.Contains(clearFn, "'/api/stream/clear'") || !strings.Contains(clearFn, "method: 'POST'") {
		t.Error("clearQueryStream does not POST /api/stream/clear, so the daemon's ring keeps " +
			"the entries and the next reconnect brings them back.")
	}

	// Another tab's Clear must be honoured without echoing a POST back, or two open
	// dashboards would clear each other in a loop.
	if !strings.Contains(app, "addEventListener('cleared'") {
		t.Error("app.js never listens for the SSE 'cleared' event; a second open dashboard " +
			"would keep showing rows the server has dropped.")
	}
	if !strings.Contains(app, "if (epoch !== streamEpoch) return;") {
		t.Error("the coalesced insert frame does not check streamEpoch, so an arrival queued " +
			"before Clear can be inserted after it.")
	}
}

// The placeholder used to be recognised by its wording. Both files now name a class instead,
// which is the only reason rewording the copy is safe.
func TestStreamPlaceholderIsIdentifiedByClass(t *testing.T) {
	app := readAsset(t, "js/app.js")
	html := readAsset(t, "index.html")

	if strings.Contains(app, "includes('Listening')") {
		t.Error("js/app.js still identifies the stream placeholder by matching the word " +
			"'Listening' in the row's text. Reword the copy in index.html and the placeholder " +
			"is never removed again — it stays pinned above the first real query, and nothing " +
			"throws or logs. Use the stream-placeholder class.")
	}
	for _, f := range []struct{ name, body string }{
		{"web/index.html", html},
		{"web/js/app.js", app},
	} {
		if !strings.Contains(f.body, "stream-placeholder") {
			t.Errorf("%s does not mention the stream-placeholder class. Both sides have to agree "+
				"on it: index.html ships the first one and app.js has to recognise it to replace "+
				"it with the first query that arrives.", f.name)
		}
	}
	if !strings.Contains(app, "querySelector('.stream-placeholder')") {
		t.Error("js/app.js no longer looks up .stream-placeholder, so nothing tells a table " +
			"holding a placeholder apart from one holding queries — the first arriving row " +
			"would be inserted above the placeholder rather than replacing it.")
	}
}

// The SSE handlers are the only writers into the stream, and there are four of them: the
// `query` event, the `history` batch, and an onmessage fallback that handles both shapes. Each
// used to write to the table itself — two of them by assigning innerHTML — which is what let
// the buffer and the view disagree. They must all go through the buffer now, or a filter change
// redraws from a record that never saw half the queries.
func TestLiveStreamHandlersFeedTheBuffer(t *testing.T) {
	body := functionBody(t, readAsset(t, "js/app.js"), "startLiveStream")

	for _, fn := range []string{"pushStreamQuery", "seedStreamQueries"} {
		if !strings.Contains(body, fn) {
			t.Errorf("startLiveStream does not call %s, so an arriving query does not reach "+
				"streamBuffer and the filter has nothing to re-filter.", fn)
		}
	}
	if strings.Contains(body, "innerHTML") {
		t.Error("startLiveStream writes innerHTML directly. That is how the history batch used " +
			"to land on screen without being recorded: the rows were visible, the buffer was " +
			"empty, and the first filter change wiped them. Hand the payload to " +
			"seedStreamQueries and let it render.")
	}
	if strings.Contains(body, "getElementById('stream-tbody')") {
		t.Error("startLiveStream looks up the table body. Nothing in the SSE path needs it — " +
			"the render path owns the table, and reaching past it is what lets the two diverge.")
	}
}

// The Clear button's write half. It is the one destructive telemetry route on the
// panel, so it is pinned end to end: unauthenticated callers get nothing, the wrong
// method gets 405, and a successful POST actually empties the ring the history
// snapshot is built from.
func TestStreamClearEndpointRequiresAuthAndEmptiesTheRing(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	h := ws.buildAdminHandler()

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(
		`{"username":"admin","password":"`+testAdminPassword+`"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login = %d — %s", loginRec.Code, loginRec.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("login carried no session token (err=%v, body=%s)", err, loginRec.Body.String())
	}

	ws.stats.PushQueryLog(database.QueryLogItem{Domain: "clear-me.example", Action: "PROXY"})
	if got := len(ws.stats.GetRecentLogs()); got != 1 {
		t.Fatalf("ring holds %d entries after one push, want 1", got)
	}

	// No token: the route must not be a free way to wipe another operator's log.
	anon := httptest.NewRequest(http.MethodPost, "/api/stream/clear", nil)
	anonRec := httptest.NewRecorder()
	h.ServeHTTP(anonRec, anon)
	if anonRec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated POST /api/stream/clear = %d, want 401", anonRec.Code)
	}
	if got := len(ws.stats.GetRecentLogs()); got != 1 {
		t.Errorf("an unauthenticated clear emptied the ring (%d entries left)", got)
	}

	// Authenticated GET: method is part of the contract, like the other write routes.
	get := httptest.NewRequest(http.MethodGet, "/api/stream/clear", nil)
	get.Header.Set("Authorization", "Bearer "+session.Token)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/stream/clear = %d, want 405", getRec.Code)
	}

	post := httptest.NewRequest(http.MethodPost, "/api/stream/clear", nil)
	post.Header.Set("Authorization", "Bearer "+session.Token)
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, post)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/stream/clear = %d — %s", postRec.Code, postRec.Body.String())
	}
	if got := ws.stats.GetRecentLogs(); len(got) != 0 {
		t.Errorf("ring holds %d entries after Clear, want 0", len(got))
	}
}

// A reconnect used to repaint the history snapshot straight through a pause: the
// 'history' listener had no pause gate, unlike the 'query' listener, so rows appeared
// while the operator had deliberately frozen the view — and immediately after a Clear
// it looked as though the clear had failed. History must seed the buffer only.
func TestStreamHistoryDoesNotRepaintWhilePaused(t *testing.T) {
	body := functionBody(t, readAsset(t, "js/app.js"), "startLiveStream")

	start := strings.Index(body, "addEventListener('history'")
	if start == -1 {
		t.Fatal("startLiveStream no longer registers a 'history' listener")
	}
	end := strings.Index(body[start:], "});")
	if end == -1 {
		t.Fatal("the 'history' listener is not closed the way the test expects")
	}
	listener := body[start : start+end]

	if !strings.Contains(listener, "isStreamPaused") {
		t.Error("the 'history' listener has no pause gate; a reconnect repaints the log while paused")
	}
	if !strings.Contains(listener, "streamBuffer = list.slice(") {
		t.Error("a paused reconnect does not seed the buffer, so Resume would show a hole")
	}
}
