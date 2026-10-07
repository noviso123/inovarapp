package webapp

import (
	"net/http"
	"path/filepath"
)

func downloadsFilesHandler() http.Handler {
	directory := filepath.Join(webRoot(), "public", "downloads")
	return http.StripPrefix("/downloads/", http.FileServer(http.Dir(directory)))
}
