package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"inovarapp/core/adapter/supabase"
)

func TestShortDocumentHandlerResolvesPrivateDocumentWithoutCaching(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	const code = "Abcdef_1234"
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-role" {
			t.Errorf("storage authorization=%q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/storage/v1/object/documentos-inovar/links/" + code + ".json":
			_, _ = fmt.Fprintf(w, `{"path":"os-orcamentos/2026-10-03/os-123.pdf","expiraEm":%d}`, now.Add(10*time.Minute).UnixMilli())
		case "/storage/v1/object/sign/documentos-inovar/os-orcamentos/2026-10-03/os-123.pdf":
			if r.Method != http.MethodPost {
				t.Errorf("sign method=%s", r.Method)
			}
			_, _ = w.Write([]byte(`{"signedURL":"/object/sign/documentos-inovar/os-orcamentos/2026-10-03/os-123.pdf?token=temporary"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer storage.Close()
	client, err := supabase.New(supabase.Config{URL: storage.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: storage.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := ShortDocumentHandler{Supabase: client, Now: func() time.Time { return now }}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/d/"+code, nil)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusFound {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("Location") != storage.URL+"/storage/v1/object/sign/documentos-inovar/os-orcamentos/2026-10-03/os-123.pdf?token=temporary" {
		t.Fatalf("headers=%v", recorder.Header())
	}
}

func TestShortDocumentHandlerRejectsInvalidAndExpiredLinks(t *testing.T) {
	for _, test := range []struct {
		name, path string
		status     int
	}{
		{name: "invalid code", path: "/d/short", status: http.StatusNotFound},
		{name: "wrong route depth", path: "/d/validcode123/extra", status: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ShortDocumentHandler{}.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d", recorder.Code, test.status)
			}
		})
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"path":"os-orcamentos/expired.pdf","expiraEm":%d}`, now.Add(-time.Second).UnixMilli())
	}))
	defer storage.Close()
	client, err := supabase.New(supabase.Config{URL: storage.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: storage.Client()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ShortDocumentHandler{Supabase: client, Now: func() time.Time { return now }}.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/d/Expired123", nil))
	if recorder.Code != http.StatusGone {
		t.Fatalf("expired link status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if !shortDocumentPath.MatchString("os-orcamentos/2026-10-03/proposta 1.pdf") || shortDocumentPath.MatchString("fotos-os/id/photo.jpg") {
		t.Fatal("short links must resolve only to OS and budget documents")
	}
}

func TestShortDocumentHandlerPreservesStorageFailureStatus(t *testing.T) {
	for _, test := range []struct {
		name       string
		transport  bool
		statusCode int
	}{
		{name: "temporary transport outage", transport: true, statusCode: http.StatusServiceUnavailable},
		{name: "storage rejected signed URL", statusCode: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
			client, err := supabase.New(supabase.Config{
				URL: "https://storage.example", AnonKey: "anon", ServiceRoleKey: "service-role",
				HTTPClient: &shortDocumentSigningFailureClient{transportFailure: test.transport},
			})
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/d/Abcdef_1234", nil)
			ShortDocumentHandler{Supabase: client, Now: func() time.Time { return now }}.ServeHTTP(recorder, request)
			if recorder.Code != test.statusCode {
				t.Fatalf("status=%d want=%d body=%q", recorder.Code, test.statusCode, recorder.Body.String())
			}
		})
	}
}

type shortDocumentSigningFailureClient struct {
	calls            int
	transportFailure bool
}

func (c *shortDocumentSigningFailureClient) Do(_ *http.Request) (*http.Response, error) {
	c.calls++
	if c.calls == 1 {
		body := `{"path":"os-orcamentos/2026/os-123.pdf","expiraEm":1791202200000}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	if c.transportFailure {
		return nil, errors.New("connection refused")
	}
	return &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"upstream failed"}`))}, nil
}
