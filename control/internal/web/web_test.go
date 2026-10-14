package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func serve(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandler(t *testing.T) {
	h := newHandler(fstest.MapFS{
		"index.html":         {Data: []byte("<html>app</html>")},
		"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
	})

	for _, target := range []string{"/", "/nodes/0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", "/assets/"} {
		r := serve(h, target)
		if r.Code != 200 || r.Body.String() != "<html>app</html>" || r.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d %q", target, r.Code, r.Header().Get("Cache-Control"))
		}
	}

	r := serve(h, "/assets/app-1a2b.js")
	if r.Body.String() != "console.log(1)" || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %q", r.Header().Get("Cache-Control"))
	}
	if csp := r.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || r.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("security headers: %q", csp)
	}
}

func TestHandlerStaysInsideDist(t *testing.T) {
	h := newHandler(fstest.MapFS{"index.html": {Data: []byte("app")}})
	// Safe outcomes: the app's index page or a 400 from net/http's ".." check.
	for _, target := range []string{"/../../etc/passwd", "/%2e%2e/%2e%2e/go.mod"} {
		r := serve(h, target)
		if b := r.Body.String(); b != "app" && r.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %q", target, r.Code, b)
		}
	}
}

func TestHandlerWithoutBuild(t *testing.T) {
	// The repository only contains dist/.gitkeep; the app is copied in by the build.
	if _, err := dist.ReadFile("dist/index.html"); err != nil && Handler() != nil {
		t.Error("handler returned although the app was not built in")
	}
}
