package webapp

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// RunServer hosts the shared Go UI and authenticated API handlers. Desktop
// shells use this same entrypoint so authorization and business rules remain
// identical to the web deployment.
func RunServer(address string) error {
	return http.ListenAndServe(address, registerServerHandlers())
}

// RunWeb starts the server used by the web deployment.
func RunWeb(address string) error { return RunServer(address) }

// StartLocalServer binds only to loopback for a desktop WebView. It returns
// the local URL and a shutdown function; it never exposes service-role keys.
func StartLocalServer() (string, func() error, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{Handler: registerServerHandlers()}
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String(), server.Close, nil
}

// LocalHandler proxies Wails asset requests to the loopback server hosting the
// same UI and Go API routes.
func LocalHandler(baseURL string) http.Handler {
	target, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || target.Scheme != "http" || target.Host == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Servidor local indisponível", http.StatusBadGateway)
		})
	}
	return httputil.NewSingleHostReverseProxy(target)
}

func webRoot() string {
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	for current := directory; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "web", "inovar.css")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return directory
		}
	}
}

func ParseServerAddress(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return ":8080"
	}
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}

// ConfiguredServerAddress preserves the local dotenv fallback used by the web
// server while allowing PORT supplied by a hosting runtime to take precedence.
func ConfiguredServerAddress(port string) string {
	if strings.TrimSpace(port) != "" {
		return ParseServerAddress(port)
	}
	return serverListenAddress()
}
