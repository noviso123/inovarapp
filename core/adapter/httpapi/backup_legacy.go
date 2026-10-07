package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/supabase"
)

// legacyLocalBackup is the localStorage archive written by the retired React
// StorageService. Its IDs are namespaced deterministically when they are not
// UUIDs, so retrying an interrupted import remains idempotent.
type legacyLocalBackup struct {
	Clients      []legacyBackupClient      `json:"clients"`
	Maintenances []legacyBackupMaintenance `json:"maintenances"`
	Profile      map[string]any            `json:"profile"`
	Version      string                    `json:"version"`
}

type legacyBackupClient struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Phone        string                  `json:"phone"`
	Document     string                  `json:"document"`
	Address      string                  `json:"address"`
	Neighborhood string                  `json:"neighborhood"`
	City         string                  `json:"city"`
	Notes        string                  `json:"notes"`
	CreatedAt    string                  `json:"createdAt"`
	Appliances   []legacyBackupAppliance `json:"appliances"`
}
type legacyBackupAppliance struct {
	ID          string `json:"id"`
	Brand       string `json:"brand"`
	Model       string `json:"model"`
	Type        string `json:"type"`
	Capacity    any    `json:"capacityBtu"`
	Room        string `json:"room"`
	Gas         string `json:"gasType"`
	Voltage     string `json:"voltage"`
	InstallDate string `json:"installDate"`
	Notes       string `json:"notes"`
}
type legacyBackupMaintenance struct {
	ID                 string         `json:"id"`
	ClientID           string         `json:"clientId"`
	ApplianceID        string         `json:"applianceId"`
	Date               string         `json:"date"`
	ScheduledDate      string         `json:"scheduledDate"`
	ScheduledTime      string         `json:"scheduledTime"`
	CompletionDate     string         `json:"completionDate"`
	ReturnDate         string         `json:"returnDate"`
	ServiceType        string         `json:"serviceType"`
	Price              float64        `json:"price"`
	Notes              string         `json:"notes"`
	PartsUsed          string         `json:"partsUsed"`
	WarrantyDays       int            `json:"warrantyDays"`
	Status             string         `json:"status"`
	PaymentMethod      string         `json:"paymentMethod"`
	Checklist          map[string]any `json:"checklist"`
	StartedAt          string         `json:"startedAt"`
	CancelledAt        string         `json:"cancelledAt"`
	CancellationReason string         `json:"cancellationReason"`
}

var legacyCapacityDigits = regexp.MustCompile(`[^0-9]`)

func decodeLegacyLocalBackup(raw json.RawMessage) (backupArchive, bool, error) {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(raw, &shape); err != nil {
		return backupArchive{}, false, err
	}
	if _, clients := shape["clients"]; !clients {
		return backupArchive{}, false, nil
	}
	if _, modern := shape["format"]; modern {
		return backupArchive{}, false, nil
	}
	var old legacyLocalBackup
	if err := json.Unmarshal(raw, &old); err != nil {
		return backupArchive{}, true, err
	}
	if old.Version != "1.0" {
		return backupArchive{}, true, errors.New("versão do backup local React não suportada")
	}
	archive := backupArchive{Format: "inovarapp-go-backup", Version: 1, Customers: []map[string]any{}, Appliances: []map[string]any{}, Services: []map[string]any{}, Appointments: []map[string]any{}, Budgets: []json.RawMessage{}, History: []map[string]any{}}
	clientIDs, applianceIDs := map[string]string{}, map[string]string{}
	for _, c := range old.Clients {
		if strings.TrimSpace(c.Name) == "" {
			return backupArchive{}, true, errors.New("backup legado contém cliente sem nome")
		}
		id, err := migratedBackupID("client", c.ID)
		if err != nil {
			return backupArchive{}, true, err
		}
		clientIDs[c.ID] = id
		customer := map[string]any{"id": id, "nome": c.Name, "ativo": true}
		addLegacyString(customer, "whatsapp", c.Phone)
		addLegacyString(customer, "cpf_cnpj", c.Document)
		addLegacyString(customer, "endereco", c.Address)
		addLegacyString(customer, "bairro", c.Neighborhood)
		addLegacyString(customer, "cidade", c.City)
		addLegacyString(customer, "observacoes", c.Notes)
		addLegacyString(customer, "created_at", c.CreatedAt)
		archive.Customers = append(archive.Customers, customer)
		for _, a := range c.Appliances {
			aid, err := migratedBackupID("appliance", a.ID)
			if err != nil {
				return backupArchive{}, true, err
			}
			applianceIDs[a.ID] = aid
			capacity, _ := strconv.Atoi(legacyCapacityDigits.ReplaceAllString(fmt.Sprint(a.Capacity), ""))
			row := map[string]any{"id": aid, "cliente_id": id, "marca": a.Brand, "tipo": a.Type, "btus": capacity, "ambiente": a.Room}
			addLegacyString(row, "modelo", a.Model)
			addLegacyString(row, "gas_tipo", a.Gas)
			addLegacyString(row, "tensao", a.Voltage)
			addLegacyString(row, "observacoes", a.Notes)
			if a.InstallDate != "" {
				if !validLegacyCivilDate(a.InstallDate) {
					return backupArchive{}, true, fmt.Errorf("aparelho %q contém data de instalação inválida", a.ID)
				}
				row["data_instalacao"] = a.InstallDate
			}
			encoded, _ := json.Marshal(row)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(encoded, &fields)
			if !validTeamApplianceFields(fields, true) {
				return backupArchive{}, true, fmt.Errorf("aparelho %q contém campos obrigatórios inválidos", a.ID)
			}
			archive.Appliances = append(archive.Appliances, row)
		}
		if c.CreatedAt != "" && !validLegacyDateTime(c.CreatedAt) {
			return backupArchive{}, true, fmt.Errorf("cliente %q contém data de cadastro inválida", c.ID)
		}
	}
	for _, m := range old.Maintenances {
		cid, ok := clientIDs[m.ClientID]
		if !ok {
			return backupArchive{}, true, fmt.Errorf("histórico %q referencia cliente ausente", m.ID)
		}
		aid := ""
		if m.ApplianceID != "" {
			var found bool
			aid, found = applianceIDs[m.ApplianceID]
			if !found {
				return backupArchive{}, true, fmt.Errorf("histórico %q referencia aparelho ausente", m.ID)
			}
		}
		serviceID, err := migratedBackupID("service", m.ID)
		if err != nil {
			return backupArchive{}, true, err
		}
		status := map[string]string{"concluido": "CONCLUIDO", "agendado": "AGENDADO", "em_andamento": "EM_ANDAMENTO", "cancelado": "CANCELADO"}[strings.ToLower(m.Status)]
		if status == "" {
			if strings.TrimSpace(m.Status) != "" {
				return backupArchive{}, true, fmt.Errorf("histórico %q contém status desconhecido", m.ID)
			}
			status = "AGENDADO" // mirrors StorageService.getMaintenances for older records
		}
		if strings.TrimSpace(m.ServiceType) == "" {
			return backupArchive{}, true, fmt.Errorf("histórico %q não informa o tipo de serviço", m.ID)
		}
		date := m.ScheduledDate
		if date == "" {
			date = m.Date
		}
		if date == "" || !validLegacyCivilDate(date) {
			return backupArchive{}, true, fmt.Errorf("histórico %q contém data inválida", m.ID)
		}
		if m.CompletionDate != "" && !validLegacyCivilDate(m.CompletionDate) {
			return backupArchive{}, true, fmt.Errorf("histórico %q contém data de conclusão inválida", m.ID)
		}
		if m.ReturnDate != "" && !validLegacyCivilDate(m.ReturnDate) {
			return backupArchive{}, true, fmt.Errorf("histórico %q contém data de retorno inválida", m.ID)
		}
		if m.ScheduledTime != "" {
			if _, err := time.Parse("15:04", m.ScheduledTime); err != nil {
				return backupArchive{}, true, fmt.Errorf("histórico %q contém horário inválido", m.ID)
			}
		}
		if m.StartedAt != "" && !validLegacyDateTime(m.StartedAt) {
			return backupArchive{}, true, fmt.Errorf("histórico %q contém data de início inválida", m.ID)
		}
		if m.CancelledAt != "" && !validLegacyDateTime(m.CancelledAt) {
			return backupArchive{}, true, fmt.Errorf("histórico %q contém data de cancelamento inválida", m.ID)
		}
		obs := m.Notes
		if m.ReturnDate != "" {
			obs = strings.TrimSpace(obs + " [PROXIMO_RETORNO:" + m.ReturnDate + "]")
		}
		if m.WarrantyDays > 0 {
			obs = strings.TrimSpace(obs + fmt.Sprintf(" [GARANTIA_DIAS:%d]", m.WarrantyDays))
		}
		if m.PartsUsed != "" {
			obs = strings.TrimSpace(obs + " Peças: " + m.PartsUsed)
		}
		if m.PaymentMethod != "" {
			obs = strings.TrimSpace(obs + " Pagamento: " + m.PaymentMethod + ".")
		}
		checklist := m.Checklist
		if checklist == nil && m.PartsUsed != "" {
			checklist = map[string]any{}
		}
		if checklist != nil {
			if m.PartsUsed != "" {
				checklist["pecasSubstituidas"] = m.PartsUsed
			}
			obs = strings.TrimSpace(obs + " [CHECKLIST:" + mustJSON(checklist) + "]")
		}
		row := map[string]any{"id": serviceID, "cliente_id": cid, "tipo": supabase.MapServiceTypeToSupabase(m.ServiceType), "descricao": m.ServiceType, "valor": m.Price, "status": status, "data_agendamento": date, "observacoes": obs}
		addLegacyString(row, "hora_agendamento", m.ScheduledTime)
		if m.StartedAt != "" {
			row["data_inicio"] = legacyDateOnly(m.StartedAt)
		}
		if m.CancelledAt != "" {
			row["data_cancelamento"] = legacyDateOnly(m.CancelledAt)
		}
		addLegacyString(row, "motivo_cancelamento", m.CancellationReason)
		if aid != "" {
			row["aparelho_id"] = aid
		}
		if m.CompletionDate != "" {
			row["data_conclusao"] = m.CompletionDate
		} else if status == "CONCLUIDO" {
			row["data_conclusao"] = m.Date
		}
		archive.Services = append(archive.Services, row)
		if status == "AGENDADO" {
			archive.Appointments = append(archive.Appointments, map[string]any{"service_id": serviceID, "cliente_id": cid, "data": date, "hora": m.ScheduledTime, "status": "AGENDADO", "observacoes": m.Notes})
		}
		if status == "CONCLUIDO" {
			history := map[string]any{"id": migratedBackupIDMust("history", m.ID), "service_id": serviceID, "cliente_id": cid, "data": m.Date, "descricao": m.ServiceType, "solucao": m.ServiceType, "problema": m.Notes, "pecas_utilizadas": m.PartsUsed, "valor": m.Price}
			if aid != "" {
				history["aparelho_id"] = aid
			}
			if checklist != nil {
				history["observacoes"] = mustJSON(checklist)
			}
			archive.History = append(archive.History, history)
		}
	}
	if len(old.Profile) > 0 {
		archive.Settings = make(map[string]json.RawMessage)
		for _, key := range []string{"name", "businessName", "cnpj", "address", "phone", "pixKey", "pixType", "defaultReturnMonths", "defaultWarrantyDays", "defaultPrice", "tiposServicosCustom", "tiposFixosRemovidos", "tiposFixosEditados", "mensagensWhats", "lembrete_intervalo_dias"} {
			if value, ok := old.Profile[key]; ok {
				encoded, err := json.Marshal(value)
				if err == nil && !containsBackupSecret(key) {
					archive.Settings[key] = encoded
				}
			}
		}
	}
	return archive, true, nil
}

var legacyBackupNamespace = uuid.MustParse("9b16327c-a4c2-5f76-8f59-2e99b2ee87d1")

func addLegacyString(row map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		row[key] = value
	}
}
func validLegacyCivilDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
func validLegacyDateTime(value string) bool {
	if validLegacyCivilDate(value) {
		return true
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
func legacyDateOnly(value string) string {
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func migratedBackupID(kind, old string) (string, error) {
	if strings.TrimSpace(old) == "" {
		return "", errors.New("backup legado contém registro sem identificador")
	}
	if id, err := uuid.Parse(old); err == nil {
		return id.String(), nil
	}
	return uuid.NewSHA1(legacyBackupNamespace, []byte(kind+":"+old)).String(), nil
}
func migratedBackupIDMust(kind, old string) string { id, _ := migratedBackupID(kind, old); return id }
func mustJSON(value any) string                    { b, _ := json.Marshal(value); return string(b) }
