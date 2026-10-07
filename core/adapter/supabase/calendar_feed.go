package supabase

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"inovarapp/core/domain"
)

var ErrCalendarFeedNotFound = errors.New("calendar feed not found")

const calendarFeedConfigPath = "/storage/v1/object/documentos-inovar/config/tecnico.json"

type calendarFeedConfig struct {
	Token string `json:"calendario_token"`
}

type calendarFeedService struct {
	ID            string  `json:"id"`
	Type          string  `json:"tipo"`
	Description   string  `json:"descricao"`
	Status        string  `json:"status"`
	ScheduledDate string  `json:"data_agendamento"`
	ScheduledTime string  `json:"hora_agendamento"`
	Value         float64 `json:"valor"`
	Notes         string  `json:"observacoes"`
	ClientID      string  `json:"cliente_id"`
}

type calendarFeedCustomer struct {
	ID           string `json:"id"`
	Name         string `json:"nome"`
	Address      string `json:"endereco"`
	Neighborhood string `json:"bairro"`
	City         string `json:"cidade"`
	WhatsApp     string `json:"whatsapp"`
}

// CalendarFeedICS reads the existing server configuration and scheduled
// records using service-role access only after validating the subscription
// token. The public feed token remains the sole access credential, matching
// the current endpoint contract.
func (c *Client) CalendarFeedICS(ctx context.Context, token string, generatedAt time.Time) (string, error) {
	configResult, err := c.ServiceRequest(ctx, calendarFeedConfigPath, RequestOptions{Method: http.MethodGet})
	if err != nil {
		return "", err
	}
	if configResult.StatusCode < http.StatusOK || configResult.StatusCode >= http.StatusMultipleChoices {
		return "", ErrCalendarFeedNotFound
	}
	var config calendarFeedConfig
	if json.Unmarshal(configResult.Body, &config) != nil || !validCalendarFeedToken(token, config.Token) {
		return "", ErrCalendarFeedNotFound
	}

	servicesResult, err := c.ServiceRequest(ctx, calendarFeedServicesPath(), RequestOptions{Method: http.MethodGet})
	if err != nil {
		return "", err
	}
	var services []calendarFeedService
	if servicesResult.StatusCode >= http.StatusOK && servicesResult.StatusCode < http.StatusMultipleChoices {
		_ = json.Unmarshal(servicesResult.Body, &services)
	}

	customersResult, err := c.ServiceRequest(ctx, "/rest/v1/customers?select=id,nome,endereco,bairro,cidade,whatsapp", RequestOptions{Method: http.MethodGet})
	if err != nil {
		return "", err
	}
	var customers []calendarFeedCustomer
	if customersResult.StatusCode >= http.StatusOK && customersResult.StatusCode < http.StatusMultipleChoices {
		_ = json.Unmarshal(customersResult.Body, &customers)
	}
	customersByID := make(map[string]calendarFeedCustomer, len(customers))
	for _, customer := range customers {
		customersByID[customer.ID] = customer
	}

	events := make([]domain.CalendarEvent, 0, len(services))
	for _, service := range services {
		start, ok := calendarFeedStart(service.ScheduledDate, service.ScheduledTime)
		if !ok {
			continue
		}
		customer := customersByID[service.ClientID]
		summary := service.Description
		if summary == "" {
			summary = service.Type
		}
		if summary == "" {
			summary = "Atendimento"
		}
		summary = strings.ReplaceAll(summary, "_", " ") + " — "
		if customer.Name == "" {
			summary += "Cliente"
		} else {
			summary += customer.Name
		}
		location := strings.Join(nonEmptyCalendarFields(customer.Address, customer.Neighborhood, customer.City), ", ")
		description := fmt.Sprintf("Serviço Inovar Refrigeração (%s). Valor: R$ %.2f. Contato: %s", service.Status, service.Value, customer.WhatsApp)
		events = append(events, domain.CalendarEvent{
			UID:         "inovar-" + service.ID + "@inovarapp",
			Summary:     summary,
			Description: description,
			Location:    location,
			Start:       start,
			End:         start.Add(2 * time.Hour),
		})
	}
	return domain.FormatCalendarFeed(events, generatedAt), nil
}

// ServeCalendarFeed preserves the public GET endpoint, response type, and
// subscription behavior while keeping privileged credentials on the server.
func (c *Client) ServeCalendarFeed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	if r.Method != http.MethodGet {
		writeCalendarFeedError(w, http.StatusMethodNotAllowed, "Método não permitido")
		return
	}
	ics, err := c.CalendarFeedICS(r.Context(), r.URL.Query().Get("token"), time.Now())
	if errors.Is(err, ErrCalendarFeedNotFound) {
		writeCalendarFeedError(w, http.StatusNotFound, "Feed de calendário não encontrado ou token inválido")
		return
	}
	if err != nil {
		writeCalendarFeedError(w, http.StatusInternalServerError, "Não foi possível carregar o calendário")
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="inovar-agenda.ics"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ics))
}

func validCalendarFeedToken(token, expected string) bool {
	if token == "" || expected == "" || len(token) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func calendarFeedServicesPath() string {
	query := url.Values{}
	query.Set("status", `in.("AGENDADO","EM_ANDAMENTO")`)
	query.Set("data_agendamento", "not.is.null")
	query.Set("select", "id,tipo,descricao,status,data_agendamento,hora_agendamento,valor,observacoes,cliente_id")
	query.Set("order", "data_agendamento.asc")
	query.Set("limit", "500")
	return "/rest/v1/services?" + query.Encode()
}

func calendarFeedStart(dateValue, timeValue string) (time.Time, bool) {
	dateValue = strings.TrimSpace(dateValue)
	if len(dateValue) > len("2006-01-02") {
		dateValue = dateValue[:len("2006-01-02")]
	}
	date, err := time.Parse("2006-01-02", dateValue)
	if err != nil {
		return time.Time{}, false
	}
	hour, minute := 9, 0
	if timeValue != "" {
		parts := strings.Split(timeValue, ":")
		if len(parts) > 0 {
			if parsed, parseErr := strconv.Atoi(parts[0]); parseErr == nil {
				hour = parsed
			}
		}
		if len(parts) > 1 {
			if parsed, parseErr := strconv.Atoi(parts[1]); parseErr == nil {
				minute = parsed
			}
		}
	}
	brt := time.FixedZone("BRT", -3*60*60)
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, brt), true
}

func nonEmptyCalendarFields(values ...string) []string {
	fields := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			fields = append(fields, value)
		}
	}
	return fields
}

func writeCalendarFeedError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
