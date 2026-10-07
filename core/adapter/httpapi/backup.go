package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
)

// BackupHandler exports the caller-visible business records as a versioned,
// account-scoped JSON archive. Every database read uses the caller JWT so RLS
// remains the authorization boundary; secrets and identity records are omitted.
type BackupHandler struct{ Supabase *supabase.Client }

type backupArchive struct {
	Format       string                     `json:"format"`
	Version      int                        `json:"version"`
	CreatedAt    string                     `json:"createdAt"`
	Customers    []map[string]any           `json:"customers"`
	Appliances   []map[string]any           `json:"appliances"`
	Services     []map[string]any           `json:"services"`
	Appointments []map[string]any           `json:"appointments"`
	Budgets      []json.RawMessage          `json:"budgets"`
	History      []map[string]any           `json:"serviceHistory"`
	Settings     map[string]json.RawMessage `json:"settings,omitempty"`
}

func (h BackupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode exportar os dados operacionais"})
		return
	}
	if r.Method == http.MethodPost {
		h.restore(w, r, caller.Token)
		return
	}
	archive := backupArchive{
		Format: "inovarapp-go-backup", Version: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	reads := []struct {
		path string
		dest any
	}{
		{"customers?select=*", &archive.Customers},
		{"air_conditioners?select=*", &archive.Appliances},
		{"services?select=*", &archive.Services},
		{"appointments?select=*", &archive.Appointments},
		{"budgets?select=*", &archive.Budgets},
		{"service_history?select=*", &archive.History},
	}
	for _, read := range reads {
		table, _, _ := strings.Cut(read.path, "?")
		rows, readErr := h.readAllRows(r, caller.Token, table)
		if readErr != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível gerar o backup completo; nenhum arquivo foi emitido."})
			return
		}
		encoded, _ := json.Marshal(rows)
		if json.Unmarshal(encoded, read.dest) != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível preparar o backup completo; nenhum arquivo foi emitido."})
			return
		}
	}
	archive.Customers = stripBackupPrivateFields(archive.Customers)
	// Settings contain integration secrets and account-wide operational tokens.
	// Export only a strict allowlist of non-secret settings.
	settingsResult, settingsErr := h.Supabase.ServiceRequest(r.Context(), "/storage/v1/object/documentos-inovar/config/tecnico.json", supabase.RequestOptions{Method: http.MethodGet})
	if settingsErr == nil && settingsResult.StatusCode == http.StatusOK {
		var stored map[string]json.RawMessage
		if json.Unmarshal(settingsResult.Body, &stored) == nil {
			archive.Settings = make(map[string]json.RawMessage)
			for _, key := range []string{"name", "businessName", "cnpj", "address", "phone", "pixKey", "pixType", "defaultReturnMonths", "defaultWarrantyDays", "defaultPrice", "tiposServicosCustom", "tiposFixosRemovidos", "tiposFixosEditados", "mensagensWhats", "lembrete_intervalo_dias"} {
				if value, ok := stored[key]; ok && !containsBackupSecret(string(value)) {
					archive.Settings[key] = value
				}
			}
		}
	}
	writeJSONAttachment(w, archive)
}

type backupRestoreResult struct {
	OK      bool           `json:"ok"`
	Message string         `json:"message,omitempty"`
	Counts  map[string]int `json:"restored,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type backupRestoreTable struct {
	table, conflict string
	rows            []map[string]any
}

func (h BackupHandler) restore(w http.ResponseWriter, r *http.Request, token string) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	raw, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: "Não foi possível ler o arquivo de backup."})
		return
	}
	legacy, recognizedLegacy, legacyErr := decodeLegacyLocalBackup(raw)
	if recognizedLegacy {
		if legacyErr != nil {
			writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: "Não foi possível converter o backup antigo: " + legacyErr.Error()})
			return
		}
		archive := legacy
		h.restoreArchive(w, r, archive, token)
		return
	}
	var archive backupArchive
	if json.Unmarshal(raw, &archive) != nil {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: "Arquivo de backup inválido ou versão não suportada."})
		return
	}
	if archive.Format != "inovarapp-go-backup" || archive.Version != 1 {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: "Arquivo de backup inválido ou versão não suportada."})
		return
	}
	h.restoreArchive(w, r, archive, token)
}

func (h BackupHandler) restoreArchive(w http.ResponseWriter, r *http.Request, archive backupArchive, token string) {
	if archive.Customers == nil || archive.Appliances == nil || archive.Services == nil || archive.Appointments == nil || archive.Budgets == nil || archive.History == nil {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: "Backup incompleto: faltam conjuntos de dados necessários."})
		return
	}
	if err := validateBackupSettings(archive.Settings); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: err.Error()})
		return
	}
	budgetRows, err := rawBackupRows(archive.Budgets)
	if err != nil {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: "O backup contém um orçamento inválido."})
		return
	}
	sets := []backupRestoreTable{
		{"customers", "id", archive.Customers},
		{"air_conditioners", "id", archive.Appliances},
		{"services", "id", archive.Services},
		{"appointments", "service_id", archive.Appointments},
		{"budgets", "id", budgetRows},
		{"service_history", "id", archive.History},
	}
	if err := validateBackupRestoreRows(sets); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, backupRestoreResult{Error: err.Error()})
		return
	}
	counts := make(map[string]int, len(sets))
	for _, set := range sets {
		for start := 0; start < len(set.rows); start += 100 {
			end := min(start+100, len(set.rows))
			batch := set.rows[start:end]
			for _, row := range batch {
				delete(row, "profile_id")
				delete(row, "user_id")
			}
			query := url.Values{"on_conflict": {set.conflict}}
			result, err := h.Supabase.UserRequest(r.Context(), token, "/rest/v1/"+set.table+"?"+query.Encode(), supabase.RequestOptions{
				Method: http.MethodPost, Body: batch, Prefer: "resolution=merge-duplicates,return=minimal",
			})
			if err != nil || result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
				writeResourceJSON(w, http.StatusBadGateway, backupRestoreResult{
					Counts: counts, Error: "A restauração parou em " + set.table + ". Os registros já aplicados foram preservados; envie o mesmo backup novamente para retomar.",
				})
				return
			}
			counts[set.table] += len(batch)
		}
	}
	if len(archive.Settings) > 0 {
		if err := h.restoreBackupSettings(r.Context(), archive.Settings); err != nil {
			writeResourceJSON(w, http.StatusBadGateway, backupRestoreResult{Counts: counts, Error: "Os dados operacionais foram restaurados, mas as configurações não puderam ser aplicadas. Reenvie o mesmo backup para retomar."})
			return
		}
	}
	writeResourceJSON(w, http.StatusOK, backupRestoreResult{OK: true, Counts: counts, Message: "Restauração concluída. Registros foram mesclados pelos IDs; nenhum dado foi excluído."})
}

func (h BackupHandler) restoreBackupSettings(ctx context.Context, incoming map[string]json.RawMessage) error {
	current, err := h.Supabase.ServiceRequest(ctx, technicianSettingsPath, supabase.RequestOptions{Method: http.MethodGet})
	config := defaultTechnicianSettings()
	if err != nil {
		return err
	}
	if current.StatusCode == http.StatusOK {
		if json.Unmarshal(current.Body, &config) != nil || config == nil {
			return errors.New("invalid settings archive")
		}
	} else if current.StatusCode != http.StatusNotFound {
		return errors.New("settings read failed")
	}
	for key, value := range incoming {
		config[key] = append(json.RawMessage(nil), value...)
	}
	result, err := h.Supabase.ServiceRequest(ctx, technicianSettingsPath, supabase.RequestOptions{Method: http.MethodPost, Headers: http.Header{"X-Upsert": []string{"true"}}, Body: config})
	if err != nil || result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		return errors.New("settings write failed")
	}
	return nil
}

func validateBackupSettings(incoming map[string]json.RawMessage) error {
	allowed := map[string]bool{}
	for _, key := range []string{"name", "businessName", "cnpj", "address", "phone", "pixKey", "pixType", "defaultReturnMonths", "defaultWarrantyDays", "defaultPrice", "tiposServicosCustom", "tiposFixosRemovidos", "tiposFixosEditados", "mensagensWhats", "lembrete_intervalo_dias"} {
		allowed[key] = true
	}
	for key, value := range incoming {
		if !allowed[key] || len(value) == 0 || !json.Valid(value) || containsBackupSecret(key) {
			return errors.New("Backup contém uma configuração inválida ou sensível.")
		}
	}
	return nil
}

func rawBackupRows(rows []json.RawMessage) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(rows))
	for _, raw := range rows {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil || row == nil {
			return nil, errors.New("invalid row")
		}
		result = append(result, row)
	}
	return result, nil
}

func validateBackupRestoreRows(sets []backupRestoreTable) error {
	for _, set := range sets {
		if len(set.rows) > 100000 {
			return errors.New("Backup excede o limite de registros por tabela.")
		}
		seen := make(map[string]bool, len(set.rows))
		for _, row := range set.rows {
			id, ok := row[set.conflict].(string)
			if !ok || strings.TrimSpace(id) == "" || seen[id] {
				return errors.New("Backup inválido: " + set.table + " contém chave ausente ou duplicada.")
			}
			seen[id] = true
			for key := range row {
				if containsBackupSecret(key) || key == "profile_id" || key == "user_id" {
					return errors.New("Backup contém campos de identidade ou credenciais e não pode ser restaurado.")
				}
			}
		}
	}
	return nil
}

func stripBackupPrivateFields(rows []map[string]any) []map[string]any {
	for _, row := range rows {
		delete(row, "profile_id")
		delete(row, "user_id")
	}
	return rows
}

func containsBackupSecret(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"token", "secret", "api_key", "password", "senha", "bearer", "authorization"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func (h BackupHandler) readAllRows(r *http.Request, token, table string) ([]json.RawMessage, error) {
	const pageSize = 500
	rows := make([]json.RawMessage, 0)
	for offset := 0; ; offset += pageSize {
		query := url.Values{"select": {"*"}, "limit": {"500"}, "offset": {itoaBackup(offset)}}
		result, err := h.Supabase.UserRequest(r.Context(), token, "/rest/v1/"+table+"?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
		if err != nil || result.StatusCode != http.StatusOK {
			return nil, http.ErrAbortHandler
		}
		var page []json.RawMessage
		if err := json.Unmarshal(result.Body, &page); err != nil || page == nil {
			return nil, http.ErrAbortHandler
		}
		rows = append(rows, page...)
		if len(page) < pageSize {
			return rows, nil
		}
		if len(rows) >= 100000 {
			return nil, http.ErrAbortHandler
		}
	}
}

func itoaBackup(n int) string {
	return strconv.Itoa(n)
}

func writeJSONAttachment(w http.ResponseWriter, archive backupArchive) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "attachment; filename=inovarapp-backup-"+time.Now().UTC().Format("2006-01-02")+".json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(archive)
}
