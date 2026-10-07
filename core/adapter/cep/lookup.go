package cep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const viaCEPURL = "https://viacep.com.br/ws/"

var cepDigitsPattern = regexp.MustCompile(`\D`)

var ErrInvalidCEP = errors.New("CEP deve conter oito dígitos")
var ErrCEPNotFound = errors.New("CEP não encontrado")

type Address struct {
	Street       string `json:"rua"`
	Neighborhood string `json:"bairro"`
	City         string `json:"cidade"`
	State        string `json:"uf"`
}

type viaCEPResponse struct {
	Street       string `json:"logradouro"`
	Neighborhood string `json:"bairro"`
	City         string `json:"localidade"`
	State        string `json:"uf"`
	Error        bool   `json:"erro"`
}

// Lookup retrieves the address associated with an eight-digit Brazilian CEP.
// An injected client/base URL keeps the adapter testable without calling ViaCEP.
func Lookup(ctx context.Context, value string, client *http.Client, baseURL string) (Address, error) {
	digits := cepDigitsPattern.ReplaceAllString(strings.TrimSpace(value), "")
	if len(digits) != 8 {
		return Address{}, ErrInvalidCEP
	}
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	if baseURL == "" {
		baseURL = viaCEPURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+digits+"/json/", nil)
	if err != nil {
		return Address{}, fmt.Errorf("build CEP request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Address{}, fmt.Errorf("request CEP: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Address{}, fmt.Errorf("CEP service returned status %d", response.StatusCode)
	}
	var payload viaCEPResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return Address{}, fmt.Errorf("decode CEP response: %w", err)
	}
	if payload.Error {
		return Address{}, ErrCEPNotFound
	}
	return Address{Street: payload.Street, Neighborhood: payload.Neighborhood, City: payload.City, State: payload.State}, nil
}

func Digits(value string) string { return cepDigitsPattern.ReplaceAllString(value, "") }

func Mask(value string) string {
	digits := Digits(value)
	if len(digits) > 8 {
		digits = digits[:8]
	}
	if len(digits) > 5 {
		return digits[:5] + "-" + digits[5:]
	}
	return digits
}
