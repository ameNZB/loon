package schedule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

type memConfigStore struct {
	mu sync.Mutex
	m  map[string]map[string]string
}

func newMemConfigStore() *memConfigStore {
	return &memConfigStore{m: map[string]map[string]string{}}
}
func (s *memConfigStore) GetJobSettings(_ context.Context, job string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.m[job] {
		out[k] = v
	}
	return out, nil
}
func (s *memConfigStore) SetJobSetting(_ context.Context, job, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v == "" {
		delete(s.m[job], k)
		return nil
	}
	if s.m[job] == nil {
		s.m[job] = map[string]string{}
	}
	s.m[job][k] = v
	return nil
}
func (s *memConfigStore) DeleteJobSetting(_ context.Context, job, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m[job], k)
	return nil
}

func TestJobConfigHandlerRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := NewRegistry()
	store := newMemConfigStore()
	job := reg.RegisterService("Search API", "read tier")
	job.DeclareConfig(store,
		JobConfigVar{Key: "cache_ttl_secs", Label: "Cache TTL", Type: JobConfigInt, Default: "90"},
	)

	r := gin.New()
	r.GET("/admin/jobs/config", JobConfigHandler(reg))
	r.POST("/admin/jobs/config", JobConfigSaveHandler(reg))

	// GET renders the form with the declared key.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/jobs/config?name=Search+API", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cache_ttl_secs") {
		t.Fatalf("GET form: code=%d body-has-key=%v", w.Code, strings.Contains(w.Body.String(), "cache_ttl_secs"))
	}

	// POST an override -> saved, and the effective value updates.
	form := url.Values{"name": {"Search API"}, "cache_ttl_secs": {"3600"}}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/jobs/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST save: code=%d want 303", w.Code)
	}
	if got := job.GetConfigInt("cache_ttl_secs"); got != 3600 {
		t.Fatalf("after save cache_ttl_secs = %d, want 3600", got)
	}

	// POST the default value -> override is cleared (no row), reverts to default.
	form = url.Values{"name": {"Search API"}, "cache_ttl_secs": {"90"}}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/jobs/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)
	if m, _ := store.GetJobSettings(context.Background(), "Search API"); len(m) != 0 {
		t.Fatalf("posting the default should leave no override row, got %v", m)
	}
	if got := job.GetConfigInt("cache_ttl_secs"); got != 90 {
		t.Fatalf("after revert cache_ttl_secs = %d, want 90", got)
	}
}
