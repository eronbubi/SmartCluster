package main

import (
	"net/http"

	"github.com/eronbubi/smartcluster/control/internal/web"
)

// webUI returns the embedded web app, or nil when the binary was built
// without it (then only the API is served).
func webUI() http.Handler { return web.Handler() }
