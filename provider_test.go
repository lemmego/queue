package queue

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemmego/api/app"
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
// authenticator. A provider with no DashboardAuth never calls it, so the
// dashboard is mounted and deliberately unreachable: it can retry, cancel and
// delete jobs, and the failure mode of the opposite default is a public button
// that clears somebody's queue.
func TestDashboardIsClosedUntilDashboardAuthIsSet(t *testing.T) {
	server := newWebServer(nil, nil, "/tasker")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasker/", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard status = %d, want 401 with no DashboardAuth", response.Code)
	}
}

// With a predicate set, the dashboard is reachable — and the predicate is
// what decides, not merely whether one exists.
func TestDashboardAuthDecidesPerRequest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		allow bool
		want  int
	}{
		{"allowed", true, http.StatusOK},
		{"refused", false, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &Provider{DashboardAuth: func(app.Context) bool { return tc.allow }}
			server := newWebServer(nil, nil, "/tasker")
			server.UseAuth(provider.dashboardGuard(nil))

			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasker/", nil))

			if response.Code != tc.want {
				t.Fatalf("dashboard status = %d, want %d", response.Code, tc.want)
			}
		})
	}
}

// The predicate receives a usable Context: this is the whole point of the
// signature, and it is what lets a project resolve its own user type.
func TestDashboardAuthSeesTheRequest(t *testing.T) {
	var seen string
	provider := &Provider{DashboardAuth: func(c app.Context) bool {
		seen = c.Header("X-Admin-Token")
		return seen == "letmein"
	}}
	server := newWebServer(nil, nil, "/tasker")
	server.UseAuth(provider.dashboardGuard(nil))

	request := httptest.NewRequest(http.MethodGet, "/tasker/", nil)
	request.Header.Set("X-Admin-Token", "letmein")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if seen != "letmein" {
		t.Fatalf("the predicate saw %q", seen)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

// Set rebinds the request onto a new context, so a guard that resolved a user
// has to pass its own request downstream. Handing on the original instead
// loses everything the predicate established — the dashboard would authorise
// a user and then serve handlers that cannot see one.
func TestDashboardGuardPassesOnWhatThePredicateEstablished(t *testing.T) {
	provider := &Provider{DashboardAuth: func(c app.Context) bool {
		c.Set("dashboard:user", "ada")
		return true
	}}

	var downstream any
	handler := provider.dashboardGuard(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		downstream = r.Context().Value("dashboard:user")
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/tasker/", nil))

	if downstream != "ada" {
		t.Fatalf("downstream saw %v, want the value the predicate set", downstream)
	}
}
