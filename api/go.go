package handler

import (
	"net/http"
	"strings"

	"inovarapp/core/ui/webapp"
)

const vercelRouteQuery = "__go_path"

// Handler dispatches Vercel's isolated /api/go function to the selected Go
// API route. vercel.json supplies the original route through __go_path.
func Handler(w http.ResponseWriter, r *http.Request) {
	routes := r.URL.Query()[vercelRouteQuery]
	if len(routes) != 1 || !allowedGoRoute(routes[0]) {
		http.NotFound(w, r)
		return
	}

	request := r.Clone(r.Context())
	urlCopy := *r.URL
	urlCopy.Path = routes[0]
	urlCopy.RawPath = ""
	query := urlCopy.Query()
	query.Del(vercelRouteQuery)
	urlCopy.RawQuery = query.Encode()
	request.URL = &urlCopy
	request.RequestURI = urlCopy.RequestURI()
	webapp.NewAPIHandler().ServeHTTP(w, request)
}

func allowedGoRoute(path string) bool {
	return strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/d/")
}
