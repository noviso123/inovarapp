package webapp

import "net/http"

var mobileWebOrigins = map[string]struct{}{
	"https://localhost":     {}, // Capacitor Android default origin.
	"capacitor://localhost": {}, // Capacitor iOS default origin.
}

func mobileCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if _, allowed := mobileWebOrigins[origin]; allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, apikey, x-client-info")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, X-Service-Order-File, X-Service-Order-URL, X-Service-Order-WhatsApp")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
