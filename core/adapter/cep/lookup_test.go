package cep

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupParsesUTF8AddressWithoutExternalNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/29060270/json/" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("accept=%q", r.Header.Get("Accept"))
		}
		_, _ = io.WriteString(w, `{"logradouro":"Rua das Flores","bairro":"Jardim","localidade":"Vitória","uf":"ES"}`)
	}))
	defer server.Close()
	address, err := Lookup(context.Background(), "29060-270", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if address.Street != "Rua das Flores" || address.Neighborhood != "Jardim" || address.City != "Vitória" || address.State != "ES" {
		t.Fatalf("address=%#v", address)
	}
}

func TestLookupRejectsInvalidAndUnknownCEPs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"erro":true}`) }))
	defer server.Close()
	if _, err := Lookup(context.Background(), "123", server.Client(), server.URL); err != ErrInvalidCEP {
		t.Fatalf("invalid CEP error=%v", err)
	}
	if _, err := Lookup(context.Background(), "00000000", server.Client(), server.URL); err != ErrCEPNotFound {
		t.Fatalf("unknown CEP error=%v", err)
	}
}

func TestMaskFormatsCEPAndTruncates(t *testing.T) {
	if got := Mask("2906027099"); got != "29060-270" {
		t.Fatalf("mask=%q", got)
	}
}
