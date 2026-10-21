package search

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stacklok/mecatl/engine/tool"
)

const youResultsJSON = `{"results":{"web":[
 {"url":"https://go.dev/doc/go1.26","title":"Go 1.26 Release Notes","description":"desc one","snippets":["matched passage"],"page_age":"2026-02-01T00:00:00"},
 {"url":"https://example.com/b","title":"Second","description":"desc two","snippets":[]},
 {"url":"https://example.com/c","title":"Third","description":"desc three"}
]}}`

type youRecorder struct {
	mu      sync.Mutex
	calls   int
	method  string
	path    string
	query   string
	count   string
	key     string
	ua      string
	body    map[string]any
	status  int
	headers map[string]string
	payload string
}

func newYouServer(t *testing.T) (*httptest.Server, *youRecorder) {
	t.Helper()
	rec := &youRecorder{payload: youResultsJSON}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.calls++
		rec.method, rec.path = r.Method, r.URL.Path
		rec.query, rec.count = r.URL.Query().Get("query"), r.URL.Query().Get("count")
		rec.key, rec.ua = r.Header.Get("X-API-Key"), r.Header.Get("User-Agent")
		rec.body = nil
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &rec.body)
		}
		for k, v := range rec.headers {
			w.Header().Set(k, v)
		}
		if rec.status != 0 {
			w.WriteHeader(rec.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, rec.payload)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestYouProvider_DefaultEndpoints(t *testing.T) {
	if got := NewYouProvider(YouConfig{}).Endpoint(); got != "https://api.you.com/v1/agents/search" {
		t.Fatalf("keyless endpoint = %q", got)
	}
	p := NewYouProvider(YouConfig{APIKey: "k"})
	if got := p.Endpoint(); got != "https://ydc-index.io/v1/search" {
		t.Fatalf("keyed endpoint = %q", got)
	}
	if !p.Keyed() || NewYouProvider(YouConfig{APIKey: "  "}).Keyed() {
		t.Fatal("Keyed() must reflect a non-blank key")
	}
}

func TestYouProvider_KeylessGET(t *testing.T) {
	srv, rec := newYouServer(t)
	p := NewYouProvider(YouConfig{KeylessEndpoint: srv.URL + "/v1/agents/search", HTTPClient: srv.Client()})
	got, err := p.Search(t.Context(), tool.SearchQuery{Query: "go release notes", Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if rec.method != http.MethodGet || rec.path != "/v1/agents/search" {
		t.Fatalf("got %s %s, want GET /v1/agents/search", rec.method, rec.path)
	}
	if rec.query != "go release notes" || rec.count != "2" {
		t.Fatalf("query=%q count=%q", rec.query, rec.count)
	}
	if rec.key != "" {
		t.Fatal("keyless mode must not send X-API-Key")
	}
	if rec.ua != "youdotcom-integration/stacklok-mecatl" {
		t.Fatalf("User-Agent = %q", rec.ua)
	}
	if len(got) != 2 {
		t.Fatalf("limit not honored: %d results", len(got))
	}
	want := tool.SearchResult{Title: "Go 1.26 Release Notes", URL: "https://go.dev/doc/go1.26", Snippet: "matched passage", Date: "2026-02-01T00:00:00", Source: "go.dev"}
	if got[0] != want {
		t.Fatalf("result[0] = %+v, want %+v", got[0], want)
	}
	if got[1].Snippet != "desc two" {
		t.Fatalf("description fallback: %q", got[1].Snippet)
	}
}

func TestYouProvider_KeyedPOST(t *testing.T) {
	srv, rec := newYouServer(t)
	p := NewYouProvider(YouConfig{APIKey: "secret-key", KeyedEndpoint: srv.URL + "/v1/search", HTTPClient: srv.Client()})
	if _, err := p.Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 5, Site: "go.dev", Freshness: "week"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if rec.method != http.MethodPost || rec.path != "/v1/search" {
		t.Fatalf("got %s %s, want POST /v1/search", rec.method, rec.path)
	}
	if rec.key != "secret-key" {
		t.Fatal("keyed mode must send X-API-Key")
	}
	if rec.query != "" {
		t.Fatal("keyed mode must not put the query in the URL")
	}
	if rec.body["query"] != "q site:go.dev" || rec.body["count"] != float64(5) || rec.body["freshness"] != "week" {
		t.Fatalf("POST body = %v", rec.body)
	}
	if rec.ua != "youdotcom-integration/stacklok-mecatl" {
		t.Fatalf("User-Agent = %q", rec.ua)
	}
}

func TestYouProvider_CredentialErrors(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden} {
		for _, keyed := range []bool{false, true} {
			srv, rec := newYouServer(t)
			rec.status = code
			cfg := YouConfig{KeylessEndpoint: srv.URL, KeyedEndpoint: srv.URL, HTTPClient: srv.Client()}
			if keyed {
				cfg.APIKey = "secret-key"
			}
			_, err := NewYouProvider(cfg).Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 1})
			var ce *YouCredentialError
			if !errors.As(err, &ce) || ce.StatusCode != code {
				t.Fatalf("HTTP %d keyed=%v: err = %v, want *YouCredentialError", code, keyed, err)
			}
			if errors.Is(err, tool.ErrSearchBackendDown) {
				t.Fatalf("HTTP %d must not be reported as backend-down", code)
			}
			if msg := err.Error(); msg == "" || strings.Contains(msg, "secret-key") {
				t.Fatalf("error message leaked the key or was empty: %q", msg)
			}
		}
	}
}

func TestYouProvider_RateLimitHonorsRetryAfter(t *testing.T) {
	srv, rec := newYouServer(t)
	rec.status = http.StatusTooManyRequests
	rec.headers = map[string]string{"Retry-After": "120"}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	p := NewYouProvider(YouConfig{KeylessEndpoint: srv.URL, HTTPClient: srv.Client(), Now: clock})

	_, err := p.Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 1})
	var rl *YouRateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 120*time.Second {
		t.Fatalf("err = %v, want *YouRateLimitError with 120s", err)
	}
	if !errors.Is(err, tool.ErrSearchBackendDown) {
		t.Fatal("429 must surface as backend-down")
	}

	// Inside the window: no egress at all.
	rec.status = 0
	now = now.Add(60 * time.Second)
	_, err = p.Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 1})
	if !errors.As(err, &rl) || rl.RetryAfter != 60*time.Second {
		t.Fatalf("inside window: err = %v, want remaining 60s", err)
	}
	if rec.calls != 1 {
		t.Fatalf("provider contacted the endpoint during Retry-After: %d calls", rec.calls)
	}

	// After the window: egress resumes.
	now = now.Add(61 * time.Second)
	if _, err := p.Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 1}); err != nil {
		t.Fatalf("after window: %v", err)
	}
	if rec.calls != 2 {
		t.Fatalf("calls = %d, want 2", rec.calls)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":        youDefaultRetryAfter,
		"garbage": youDefaultRetryAfter,
		"0":       youDefaultRetryAfter,
		"5":       5 * time.Second,
		"999999":  youMaxRetryAfter,
		now.Add(90 * time.Second).Format(http.TimeFormat): 90 * time.Second,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestYouProvider_BackendDown(t *testing.T) {
	cases := map[string]func(*youRecorder){
		"5xx":       func(r *youRecorder) { r.status = http.StatusBadGateway },
		"malformed": func(r *youRecorder) { r.payload = "not json" },
		"empty":     func(r *youRecorder) { r.payload = `{"results":{"web":[]}}` },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			srv, rec := newYouServer(t)
			mutate(rec)
			_, err := NewYouProvider(YouConfig{KeylessEndpoint: srv.URL, HTTPClient: srv.Client()}).
				Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 1})
			if !errors.Is(err, tool.ErrSearchBackendDown) {
				t.Fatalf("err = %v, want ErrSearchBackendDown", err)
			}
		})
	}
}

func TestYouProvider_NoRedirects(t *testing.T) {
	srv, rec := newYouServer(t)
	rec.status = http.StatusFound
	rec.headers = map[string]string{"Location": "https://elsewhere.invalid/"}
	_, err := NewYouProvider(YouConfig{KeylessEndpoint: srv.URL, HTTPClient: srv.Client()}).
		Search(t.Context(), tool.SearchQuery{Query: "q", Limit: 1})
	if !errors.Is(err, tool.ErrSearchBackendDown) || rec.calls != 1 {
		t.Fatalf("redirect must not be followed: err=%v calls=%d", err, rec.calls)
	}
}
