package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizePhoneUsesLegacyBrazilRules(t *testing.T) {
	cases := []struct {
		input string
		want  string
		valid bool
	}{
		{"(27) 99999-1234", "5527999991234", true},
		{"+55 27 99999-1234", "5527999991234", true},
		{"1 415 555 0100", "5514155550100", true},
		{"12345", "12345", false},
	}
	for _, test := range cases {
		got, valid := NormalizePhone(test.input)
		if got != test.want || valid != test.valid {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q, %v", test.input, got, valid, test.want, test.valid)
		}
	}
}

func TestSenderUsesWhatsAppGoServiceAndPreservesUTF8(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/inovar/messages" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer proprio-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["to"] != "5527999991234@s.whatsapp.net" || body["text"] != "Olá, João!" {
			t.Errorf("body = %#v", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := Sender{HTTPClient: server.Client()}
	err := sender.SendText(context.Background(), Config{
		ProprioURL: server.URL, ProprioToken: "proprio-key", ProprioSession: "inovar",
	}, "(27) 99999-1234", "Olá, João!")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSenderSendsPDFByProprioWithCaption(t *testing.T) {
	var gotCaption, gotTo, gotFilename, gotContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/inovar/messages" || r.Method != http.MethodPost {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatal(err)
		}
		gotTo = r.FormValue("to")
		gotCaption = r.FormValue("caption")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		gotFilename = header.Filename
		content, _ := io.ReadAll(file)
		gotContent = string(content)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	// Serve o documento baixado pelo sender na mesma URL assinada do teste.
	doc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("%PDF-1.7 documento de teste"))
	}))
	defer doc.Close()

	sender := Sender{HTTPClient: doc.Client()}
	delivery, err := sender.SendDocumentWithResult(context.Background(), Config{ProprioURL: server.URL, ProprioToken: "proprio-key", ProprioSession: "inovar"}, "(27) 99999-1234", "Sua OS foi concluída", doc.URL+"/signed.pdf", "OS_João.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !delivery.Attached || delivery.LinkSent {
		t.Fatalf("delivery=%+v; expected PDF attachment", delivery)
	}
	if gotTo != "5527999991234@s.whatsapp.net" || gotCaption != "Sua OS foi concluída" || gotFilename != "OS_João.pdf" || !strings.HasPrefix(gotContent, "%PDF-") {
		t.Errorf("multipart recebido: to=%q caption=%q filename=%q content=%q", gotTo, gotCaption, gotFilename, gotContent[:12])
	}
}

func TestSenderFallsBackToSignedLinkWhenProprioMediaFails(t *testing.T) {
	var text string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/documento-indisponivel.pdf" {
			_, _ = w.Write([]byte("%PDF-1.7 documento de teste"))
			return
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		text = body["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	// Upload multipart falha (502); o fallback envia o link como texto.
	sender := Sender{HTTPClient: server.Client()}
	delivery, err := sender.SendDocumentWithResult(context.Background(), Config{ProprioURL: server.URL, ProprioToken: "proprio-key", ProprioSession: "inovar"}, "27999991234", "Ordem concluída", server.URL+"/documento-indisponivel.pdf", "OS.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Attached || !delivery.LinkSent {
		t.Fatalf("delivery=%+v; expected signed-link fallback", delivery)
	}
	if text != "Ordem concluída\n\n📎 Documento: "+server.URL+"/documento-indisponivel.pdf" {
		t.Fatalf("fallback text=%q", text)
	}
}
