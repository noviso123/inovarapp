package webapp

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Derive the cache identity from the actual bundle, even when APP_VERSION has
// not changed. Every frontend update must replace the old offline resources.
func frontendAssetVersion(root, label string) string {
	hash := sha256.New()
	packaged, _ := packagedWebFS()
	for _, name := range []string{"app.wasm", "inovar.css", "clean-auth-query.js"} {
		data, err := os.ReadFile(filepath.Join(root, "web", name))
		if err != nil && packaged != nil {
			data, _ = fs.ReadFile(packaged, name)
		}
		hash.Write([]byte(name))
		hash.Write(data)
	}
	version := hex.EncodeToString(hash.Sum(nil))[:20]
	if label = strings.TrimSpace(label); label != "" {
		return label + "-" + version
	}
	return "bundle-" + version
}

type pwaResources struct {
	files   http.Handler
	version string
}

func newPWAResources(root string, appVersion ...string) pwaResources {
	version := ""
	if len(appVersion) > 0 {
		version = strings.TrimSpace(appVersion[0])
	}
	webDirectory := filepath.Join(root, "web")
	if _, err := os.Stat(filepath.Join(webDirectory, "inovar.css")); err == nil {
		return pwaResources{files: http.FileServer(http.Dir(webDirectory)), version: version}
	}
	assets, err := packagedWebFS()
	if err != nil {
		return pwaResources{files: http.NotFoundHandler(), version: version}
	}
	return pwaResources{files: http.FileServer(http.FS(assets)), version: version}
}

func (r pwaResources) Resolve(location string) string {
	if (location == "/web/app.wasm" || location == "/web/inovar.css") && r.version != "" {
		return location + "?v=" + url.QueryEscape(r.version)
	}
	return location
}

func (r pwaResources) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/web/") {
		http.StripPrefix("/web/", r.files).ServeHTTP(w, request)
		return
	}
	http.NotFound(w, request)
}
