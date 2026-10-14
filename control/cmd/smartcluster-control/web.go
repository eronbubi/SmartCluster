package main

import "net/http"

// webUI returns the handler for the embedded web app. The dashboard is added
// in the next step of milestone a; until then only the API is served.
func webUI() http.Handler { return nil }
