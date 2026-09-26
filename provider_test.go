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

	// Authentication has to be installed before routing can be observed at
	// all: the management server is fail-closed, and while it is closed it
	// answers 401 for every path, matched or not. A pass-through stands in
	// for whatever a project would really use.
	server := newWebServer(nil, nil, "/jobs")
	server.UseAuth(func(next http.Handler) http.Handler { return next })

	request := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard status at the resolved prefix = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/tasker/", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unresolved prefix status = %d", response.Code)
	}
}

// The management server refuses every request until UseAuth installs an
// authenticator, and newWebServer never calls it. So the dashboard a project
// gets at TASKER_ROUTE_PREFIX is mounted and permanently unreachable.
//
// This is a deliberate fail-closed default in tasker — an unauthenticated
// dashboard can retry, cancel and clear jobs — but the queue provider has no
// way to configure it yet, so the feature is effectively unavailable. Pinned
// here so the gap is recorded rather than rediscovered, and so this test
// fails the moment someone wires authentication up and makes it reachable.
func TestDashboardIsUnreachableUntilAuthIsConfigured(t *testing.T) {
	server := newWebServer(nil, nil, "/tasker")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasker/", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard status = %d, want 401; if authentication is now "+
			"configurable, update this test and the provider together", response.Code)
	}
}
