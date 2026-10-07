package googlecalendar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
)

const eventsURL = "https://www.googleapis.com/calendar/v3/calendars/primary/events"
const tokenURL = "https://oauth2.googleapis.com/token"

type Endpoints struct{ CalendarBase, Token string }

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Connection struct {
	AccessToken  string            `json:"accessToken,omitempty"`
	RefreshToken string            `json:"refreshToken,omitempty"`
	ExpiresAt    int64             `json:"expiresAt,omitempty"`
	Events       map[string]string `json:"events,omitempty"`
	LastSync     string            `json:"lastSync,omitempty"`
	Disconnected bool              `json:"disconnected,omitempty"`
}

type SyncResult struct {
	Connected bool   `json:"connected"`
	Changed   int    `json:"changed,omitempty"`
	LastSync  string `json:"lastSync,omitempty"`
}

type serviceRow struct {
	ID            string `json:"id"`
	CustomerID    string `json:"cliente_id"`
	Type          string `json:"tipo"`
	Description   string `json:"descricao"`
	Status        string `json:"status"`
	ScheduledDate string `json:"data_agendamento"`
	ScheduledTime string `json:"hora_agendamento"`
}

type customerRow struct {
	ID           string `json:"id"`
	Name         string `json:"nome"`
	Address      string `json:"endereco"`
	Neighborhood string `json:"bairro"`
	City         string `json:"cidade"`
	WhatsApp     string `json:"whatsapp"`
}

type eventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type privateProperties struct {
	ServiceID string `json:"inovarServiceId"`
}

type eventPayload struct {
	Summary            string        `json:"summary"`
	Description        string        `json:"description"`
	Location           string        `json:"location"`
	Start              eventDateTime `json:"start"`
	End                eventDateTime `json:"end"`
	ExtendedProperties struct {
		Private privateProperties `json:"private"`
	} `json:"extendedProperties"`
}

func EventID(serviceID string) string {
	sum := sha256.Sum256([]byte(serviceID))
	return "inovar" + hex.EncodeToString(sum[:])[:40]
}

func EventPayload(service serviceRow, customer customerRow) (eventPayload, error) {
	day := strings.TrimSpace(service.ScheduledDate)
	if len(day) > 10 {
		day = day[:10]
	}
	clock := strings.TrimSpace(service.ScheduledTime)
	if clock == "" {
		clock = "09:00"
	}
	if len(clock) > 5 {
		clock = clock[:5]
	}
	if len(day) != 10 || len(clock) != 5 {
		return eventPayload{}, errors.New("Data ou horário de agendamento inválido.")
	}
	start, err := time.Parse("2006-01-02 15:04 -07:00", day+" "+clock+" -03:00")
	if err != nil {
		return eventPayload{}, errors.New("Data ou horário de agendamento inválido.")
	}
	serviceName := service.Description
	if serviceName == "" {
		serviceName = service.Type
	}
	if serviceName == "" {
		serviceName = "Atendimento"
	}
	customerName := customer.Name
	if customerName == "" {
		customerName = "Cliente"
	}
	value := eventPayload{
		Summary:     "Inovar: " + serviceName + " — " + customerName,
		Description: fmt.Sprintf("OS %s\nSituação: %s\nContato: %s", service.ID, service.Status, customer.WhatsApp),
		Location:    strings.Join(nonEmpty(customer.Address, customer.Neighborhood, customer.City), ", "),
		Start:       eventDateTime{DateTime: start.UTC().Format(time.RFC3339), TimeZone: "America/Sao_Paulo"},
		End:         eventDateTime{DateTime: start.Add(2 * time.Hour).UTC().Format(time.RFC3339), TimeZone: "America/Sao_Paulo"},
	}
	value.ExtendedProperties.Private.ServiceID = service.ID
	return value, nil
}

func Sync(ctx context.Context, client *supabase.Client, caller supabase.Caller, connection *Connection, google HTTPDoer, clientID, clientSecret string, now time.Time, configured ...Endpoints) (SyncResult, error) {
	if connection == nil || connection.Disconnected {
		return SyncResult{Connected: false}, nil
	}
	if google == nil {
		google = &http.Client{Timeout: 15 * time.Second}
	}
	endpoint := Endpoints{CalendarBase: eventsURL, Token: tokenURL}
	if len(configured) > 0 {
		if configured[0].CalendarBase != "" {
			endpoint.CalendarBase = configured[0].CalendarBase
		}
		if configured[0].Token != "" {
			endpoint.Token = configured[0].Token
		}
	}
	token, err := refreshAccessToken(ctx, google, connection, clientID, clientSecret, now, endpoint.Token)
	if err != nil {
		return SyncResult{}, err
	}
	var services []serviceRow
	serviceResult, err := client.UserRequest(ctx, caller.Token, "/rest/v1/services?select=id,cliente_id,tipo,descricao,status,data_agendamento,hora_agendamento&limit=1000", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return SyncResult{}, err
	}
	if serviceResult.StatusCode != http.StatusOK || json.Unmarshal(serviceResult.Body, &services) != nil {
		return SyncResult{}, errors.New("Não foi possível ler a agenda. Nenhum evento foi removido.")
	}
	var customers []customerRow
	customerResult, err := client.UserRequest(ctx, caller.Token, "/rest/v1/customers?select=id,nome,endereco,bairro,cidade,whatsapp&limit=1000", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return SyncResult{}, err
	}
	if customerResult.StatusCode != http.StatusOK || json.Unmarshal(customerResult.Body, &customers) != nil {
		return SyncResult{}, errors.New("Não foi possível ler a agenda. Nenhum evento foi removido.")
	}
	if len(services) >= 1000 || len(customers) >= 1000 {
		return SyncResult{}, errors.New("A agenda excedeu o limite de sincronização. Nenhum evento foi removido.")
	}
	clients := make(map[string]customerRow, len(customers))
	for _, customer := range customers {
		clients[customer.ID] = customer
	}
	previous := connection.Events
	if previous == nil {
		previous = map[string]string{}
	}
	next := make(map[string]string, len(previous))
	for key, value := range previous {
		next[key] = value
	}
	active := make(map[string]bool)
	changed := 0
	for _, service := range services {
		if (service.Status != "AGENDADO" && service.Status != "EM_ANDAMENTO") || strings.TrimSpace(service.ScheduledDate) == "" {
			continue
		}
		active[service.ID] = true
		payload, err := EventPayload(service, clients[service.CustomerID])
		if err != nil {
			return SyncResult{}, err
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return SyncResult{}, err
		}
		fingerprint := sha256.Sum256(encoded)
		fingerprintText := hex.EncodeToString(fingerprint[:])
		if previous[service.ID] == fingerprintText {
			continue
		}
		eventID := EventID(service.ID)
		response, err := googleCall(ctx, google, token, http.MethodPut, endpoint.CalendarBase+"/"+eventID, payload)
		if err != nil {
			return SyncResult{}, err
		}
		if response.StatusCode == http.StatusNotFound {
			payloadWithID := map[string]any{}
			_ = json.Unmarshal(encoded, &payloadWithID)
			payloadWithID["id"] = eventID
			response, err = googleCall(ctx, google, token, http.MethodPost, endpoint.CalendarBase, payloadWithID)
			if err != nil {
				return SyncResult{}, err
			}
			if response.StatusCode == http.StatusConflict {
				response, err = googleCall(ctx, google, token, http.MethodPut, endpoint.CalendarBase+"/"+eventID, payload)
			}
			if err != nil {
				return SyncResult{}, err
			}
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return SyncResult{}, errors.New("O Google não confirmou a atualização de um agendamento. Tente sincronizar novamente.")
		}
		next[service.ID] = fingerprintText
		changed++
	}
	for serviceID := range previous {
		if active[serviceID] {
			continue
		}
		response, err := googleCall(ctx, google, token, http.MethodDelete, endpoint.CalendarBase+"/"+EventID(serviceID), nil)
		if err != nil {
			return SyncResult{}, err
		}
		if response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusGone && (response.StatusCode < 200 || response.StatusCode >= 300) {
			return SyncResult{}, errors.New("Não foi possível remover um evento encerrado. Tente sincronizar novamente.")
		}
		delete(next, serviceID)
		changed++
	}
	connection.Events, connection.LastSync = next, now.UTC().Format(time.RFC3339Nano)
	if err := client.StoreGoogleConnection(ctx, caller.UserID, connection); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Connected: true, Changed: changed, LastSync: connection.LastSync}, nil
}

func refreshAccessToken(ctx context.Context, client HTTPDoer, connection *Connection, clientID, clientSecret string, now time.Time, endpoint string) (string, error) {
	if connection.ExpiresAt > now.Add(time.Minute).UnixMilli() && connection.AccessToken != "" {
		return connection.AccessToken, nil
	}
	if connection.RefreshToken == "" || clientID == "" || clientSecret == "" {
		return "", errors.New("Reconecte o Google Agenda para renovar a autorização.")
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {connection.RefreshToken}, "client_id": {clientID}, "client_secret": {clientSecret}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil || response.StatusCode < 200 || response.StatusCode >= 300 || payload.AccessToken == "" {
		return "", errors.New("O Google expirou ou revogou a autorização. Reconecte a agenda.")
	}
	connection.AccessToken, connection.ExpiresAt = payload.AccessToken, now.Add(time.Duration(payload.ExpiresIn)*time.Second).UnixMilli()
	return connection.AccessToken, nil
}

func googleCall(ctx context.Context, client HTTPDoer, token, method, target string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		response.Body.Close()
		return nil, errors.New("Reconecte a agenda e autorize o acesso ao Google Calendar.")
	}
	if response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusConflict && response.StatusCode != http.StatusGone {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	}
	response.Body.Close()
	response.Body = io.NopCloser(strings.NewReader(""))
	return response, nil
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
