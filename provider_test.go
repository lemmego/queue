package queue

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutePatternAndWebFactoryUseResolvedPrefix(t *testing.T) {
	if got := routePattern("/jobs"); got != "/jobs/" {
		t.Fatalf("route pattern = %q", got)
	}

	server := newWebServer(nil, nil, "/jobs")
	request := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/tasker/", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unresolved prefix status = %d", response.Code)
	}
}
