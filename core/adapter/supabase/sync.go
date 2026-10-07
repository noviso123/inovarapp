package supabase

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"inovarapp/core/domain"
)

type SupabaseAirConditioner struct {
	ID               string   `json:"id"`
	ClientID         string   `json:"cliente_id"`
	Type             string   `json:"tipo,omitempty"`
	Brand            string   `json:"marca"`
	Model            string   `json:"modelo,omitempty"`
	BTU              *float64 `json:"btus,omitempty"`
	SerialNumber     string   `json:"numero_serie,omitempty"`
	Room             string   `json:"ambiente,omitempty"`
	InstallationSite string   `json:"local_instalacao,omitempty"`
	InstallDate      string   `json:"data_instalacao,omitempty"`
	LastMaintenance  string   `json:"ultima_manutencao,omitempty"`
	Notes            string   `json:"observacoes,omitempty"`
	GasType          string   `json:"gas_tipo,omitempty"`
	Voltage          string   `json:"tensao,omitempty"`
}

type SupabaseCustomer struct {
	ID           string                   `json:"id"`
	ProfileID    string                   `json:"profile_id,omitempty"`
	Name         string                   `json:"nome"`
	Document     string                   `json:"cpf_cnpj,omitempty"`
	WhatsApp     string                   `json:"whatsapp,omitempty"`
	Address      string                   `json:"endereco,omitempty"`
	Number       string                   `json:"numero,omitempty"`
	Complement   string                   `json:"complemento,omitempty"`
	Neighborhood string                   `json:"bairro,omitempty"`
	City         string                   `json:"cidade,omitempty"`
	State        string                   `json:"estado,omitempty"`
	Notes        string                   `json:"observacoes,omitempty"`
	CreatedAt    string                   `json:"created_at,omitempty"`
	Active       bool                     `json:"ativo"`
	Appliances   []SupabaseAirConditioner `json:"appliances,omitempty"`
}

type SupabaseServiceRow struct {
	ID                 string  `json:"id"`
	ClientID           string  `json:"cliente_id"`
	ApplianceID        string  `json:"aparelho_id,omitempty"`
	TechnicianID       string  `json:"tecnico_id,omitempty"`
	Type               string  `json:"tipo"`
	Description        string  `json:"descricao,omitempty"`
	Problem            string  `json:"problema,omitempty"`
	RequestDate        string  `json:"data_solicitacao,omitempty"`
	ScheduledDate      string  `json:"data_agendamento,omitempty"`
	ScheduledTime      string  `json:"hora_agendamento,omitempty"`
	StartedDate        string  `json:"data_inicio,omitempty"`
	CompletionDate     string  `json:"data_conclusao,omitempty"`
	CancellationDate   string  `json:"data_cancelamento,omitempty"`
	CancellationReason string  `json:"motivo_cancelamento,omitempty"`
	Status             string  `json:"status"`
	Value              float64 `json:"valor,omitempty"`
	Notes              string  `json:"observacoes,omitempty"`
}

var (
	completionMarkerPattern = regexp.MustCompile(`\[DATA_CONCLUSAO:(\d{4}-\d{2}-\d{2})\]`)
	startMarkerPattern      = regexp.MustCompile(`\[DATA_INICIO:(\d{4}-\d{2}-\d{2})\]`)
	cancelMarkerPattern     = regexp.MustCompile(`\[DATA_CANCELAMENTO:(\d{4}-\d{2}-\d{2})\]`)
	cancelReasonPattern     = regexp.MustCompile(`\[MOTIVO_CANCELAMENTO:([^\]]+)\]`)
	warrantyMarkerPattern   = regexp.MustCompile(`\[GARANTIA_DIAS:(\d+)\]`)
	returnMarkerPattern     = regexp.MustCompile(`\[PROXIMO_RETORNO:(\d{4}-\d{2}-\d{2})\]`)
	laborMarkerPattern      = regexp.MustCompile(`\[MAO_OBRA:(\d+(\.\d+)?)\]`)
	partsMarkerPattern      = regexp.MustCompile(`\[PECAS:(\d+(\.\d+)?)\]`)
	checklistMarkerPattern  = regexp.MustCompile(`(?s)\[CHECKLIST:(\{.*?\})\]`)
	internalMarkersPattern  = regexp.MustCompile(`\[(GARANTIA_DIAS|DATA_CONCLUSAO|DATA_INICIO|DATA_CANCELAMENTO|MOTIVO_CANCELAMENTO|PROXIMO_RETORNO|MAO_OBRA|PECAS):[^\]]+\]`)
)

// MapSupabaseCustomerToLocal applies the defaults used while importing rows to the UI.
func MapSupabaseCustomerToLocal(row SupabaseCustomer, now time.Time) domain.Client {
	createdAt := row.CreatedAt
	if createdAt == "" {
		createdAt = now.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	city := row.City
	if city == "" {
		city = "Serra"
	}
	appliances := make([]domain.Appliance, 0, len(row.Appliances))
	for _, appliance := range row.Appliances {
		appliances = append(appliances, MapSupabaseAirConditionerToLocal(appliance, row.ID))
	}
	return domain.Client{
		ID: row.ID, Name: row.Name, Phone: row.WhatsApp, Document: stringPointer(row.Document),
		Address: stringPointer(row.Address), Neighborhood: stringPointer(row.Neighborhood), City: stringPointer(city),
		Notes: stringPointer(row.Notes), CreatedAt: createdAt, Appliances: appliances,
	}
}

func MapSupabaseAirConditionerToLocal(row SupabaseAirConditioner, clientID string) domain.Appliance {
	typeName := row.Type
	if typeName == "" {
		typeName = string(domain.ApplianceSplit)
	}
	capacity := "12.000"
	if row.BTU != nil && *row.BTU != 0 {
		capacity = formatBrazilianNumber(*row.BTU)
	}
	room := row.Room
	if room == "" {
		room = "Sala de Estar"
	}
	return domain.Appliance{
		ID: row.ID, ClientID: clientID, Brand: row.Brand, Model: stringPointer(row.Model),
		Type: domain.ApplianceType(typeName), CapacityBTU: capacity, Room: room,
		SerialNumber: row.SerialNumber, InstallationSite: row.InstallationSite,
		GasType: refrigerantPointerOrDefault(row.GasType), Voltage: voltagePointerOrDefault(row.Voltage), InstallDate: nonEmptyStringPointer(row.InstallDate), Notes: nonEmptyStringPointer(row.Notes),
	}
}

func MapSupabaseCustomersToLocal(rows []SupabaseCustomer, now time.Time) []domain.Client {
	clients := make([]domain.Client, 0, len(rows))
	for _, row := range rows {
		clients = append(clients, MapSupabaseCustomerToLocal(row, now))
	}
	return clients
}

func MapSupabaseServiceTypeToLocal(serviceType string) domain.ServiceType {
	switch serviceType {
	case "LIMPEZA":
		return domain.ServiceCleaning
	case "INSTALACAO":
		return domain.ServiceInstallation
	case "MANUTENCAO_CORRETIVA":
		return domain.ServiceCorrective
	case "MANUTENCAO_PREVENTIVA":
		return domain.ServicePreventive
	case "RECARGA_GAS":
		return domain.ServiceRefrigerant
	case "AVALIACAO":
		return domain.ServiceTechnicalAssessment
	case "OUTRO":
		return domain.ServiceOther
	default:
		return domain.ServiceType(domain.NormalizeServiceName(serviceType))
	}
}

func MapServiceTypeToSupabase(serviceType string) string {
	switch domain.NormalizeServiceName(serviceType) {
	case string(domain.ServiceCleaning):
		return "LIMPEZA"
	case string(domain.ServiceInstallation):
		return "INSTALACAO"
	case string(domain.ServiceCorrective), "Conserto / Carga Gás":
		return "MANUTENCAO_CORRETIVA"
	case string(domain.ServiceRefrigerant):
		return "RECARGA_GAS"
	case string(domain.ServicePreventive):
		return "MANUTENCAO_PREVENTIVA"
	case string(domain.ServiceTechnicalAssessment):
		return "AVALIACAO"
	default:
		return "OUTRO"
	}
}

// MapSupabaseServiceToLocal restores lifecycle data stored in columns and legacy observation markers.
func MapSupabaseServiceToLocal(row SupabaseServiceRow, profile domain.TechnicianProfile, now time.Time) domain.MaintenanceRecord {
	markers := row.Notes
	observations := row.Notes
	if observations == "" {
		observations = row.Problem
	}
	if observations == "" {
		observations = row.Description
	}

	completionMarker := firstCapture(completionMarkerPattern, markers)
	completionDate := dateOnly(row.CompletionDate)
	if completionDate == "" {
		completionDate = completionMarker
	}
	scheduledDate := row.ScheduledDate
	if scheduledDate == "" && row.RequestDate != "" {
		scheduledDate = strings.Split(row.RequestDate, "T")[0]
	}
	isDone := row.Status == "CONCLUIDO"
	if isDone && completionDate == "" {
		completionDate = scheduledDate
	}
	baseDate := completionDate
	if baseDate == "" {
		baseDate = scheduledDate
	}
	if baseDate == "" {
		baseDate = now.Format("2006-01-02")
	}

	startDate := dateOnly(row.StartedDate)
	if startDate == "" {
		startDate = firstCapture(startMarkerPattern, markers)
	}
	cancellationDate := dateOnly(row.CancellationDate)
	if cancellationDate == "" {
		cancellationDate = firstCapture(cancelMarkerPattern, markers)
	}
	cancellationReason := row.CancellationReason
	if cancellationReason == "" {
		cancellationReason = firstCapture(cancelReasonPattern, markers)
	}

	var checklist *domain.ChecklistData
	if match := checklistMarkerPattern.FindStringSubmatch(markers); len(match) > 1 {
		var parsed domain.ChecklistData
		if json.Unmarshal([]byte(match[1]), &parsed) == nil {
			checklist = &parsed
		}
	}
	partsUsed := ""
	if checklist != nil {
		partsUsed = checklist.PartsReplaced
	}
	cleanNotes := internalMarkersPattern.ReplaceAllString(observations, "")
	cleanNotes = checklistMarkerPattern.ReplaceAllString(cleanNotes, "")
	cleanNotes = collapseJavaScriptWhitespace(cleanNotes)

	returnDate := ""
	if isDone {
		returnDate = firstCapture(returnMarkerPattern, markers)
		if returnDate == "" {
			months := profile.DefaultReturnMonths
			if months == 0 {
				months = 6
			}
			returnDate = addMonthsISO(baseDate, months, now.Location(), now)
		}
	}

	serviceType := row.Type
	if row.Type == "OUTRO" && row.Description != "" {
		serviceType = row.Description
	} else {
		serviceType = string(MapSupabaseServiceTypeToLocal(row.Type))
	}
	serviceType = domain.NormalizeServiceName(serviceType)

	warrantyDays := 0
	warrantyFound := false
	if raw := firstCapture(warrantyMarkerPattern, markers); raw != "" {
		warrantyDays, _ = strconv.Atoi(raw)
		warrantyFound = true
	}
	if !warrantyFound {
		warrantyDays = profile.DefaultWarrantyDays
	}
	if !warrantyFound && warrantyDays == 0 {
		warrantyDays = 90
	}

	status := domain.MaintenanceScheduled
	switch row.Status {
	case "CONCLUIDO":
		status = domain.MaintenanceCompleted
	case "CANCELADO":
		status = domain.MaintenanceCancelled
	case "EM_ANDAMENTO":
		status = domain.MaintenanceInProgress
	}

	completedAt := domain.NullString()
	if isDone && completionDate != "" {
		completedAt = domain.StringValue(completionDate + "T12:00:00")
	}
	startedAt := domain.NullString()
	if startDate != "" {
		startedAt = domain.StringValue(startDate + "T12:00:00")
	}
	cancelledAt := domain.NullString()
	if cancellationDate != "" {
		cancelledAt = domain.StringValue(cancellationDate + "T12:00:00")
	}

	var laborPrice, partsPrice *float64
	if value := firstCapture(laborMarkerPattern, markers); value != "" {
		laborPrice = parseFloatPointer(value)
	}
	if value := firstCapture(partsMarkerPattern, markers); value != "" {
		partsPrice = parseFloatPointer(value)
	}
	var notes *string
	if cleanNotes != "" {
		notes = &cleanNotes
	}
	completionField := ""
	if isDone {
		completionField = completionDate
	}
	return domain.MaintenanceRecord{
		ID: row.ID, ClientID: row.ClientID, ApplianceID: row.ApplianceID,
		Date: baseDate, ScheduledDate: scheduledDate, ScheduledTime: row.ScheduledTime,
		CompletionDate: completionField, CompletedAt: completedAt,
		StartedAt: startedAt, CancelledAt: cancelledAt, CancellationReason: cancellationReason,
		ReturnDate: returnDate, ServiceType: domain.ServiceType(serviceType), Price: numberOrZeroFloat(row.Value),
		LaborPrice: laborPrice, PartsPrice: partsPrice, PartsUsed: partsUsed, PaymentMethod: domain.PaymentPIX,
		WarrantyDays: warrantyDays, Notes: notes, Checklist: checklist, Status: status,
	}
}

func MapSupabaseServicesToLocal(rows []SupabaseServiceRow, profile domain.TechnicianProfile, now time.Time) []domain.MaintenanceRecord {
	services := make([]domain.MaintenanceRecord, 0, len(rows))
	for _, row := range rows {
		services = append(services, MapSupabaseServiceToLocal(row, profile, now))
	}
	return services
}

func firstCapture(pattern *regexp.Regexp, value string) string {
	match := pattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func dateOnly(value string) string {
	if len(value) < 10 {
		return ""
	}
	return value[:10]
}

func stringPointer(value string) *string {
	copy := value
	return &copy
}

func nonEmptyStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return stringPointer(value)
}

func refrigerantPointer(value domain.RefrigerantType) *domain.RefrigerantType {
	copy := value
	return &copy
}

func refrigerantPointerOrDefault(value string) *domain.RefrigerantType {
	switch domain.RefrigerantType(value) {
	case domain.Refrigerant410A, domain.Refrigerant32, domain.Refrigerant22, domain.RefrigerantOther:
		return refrigerantPointer(domain.RefrigerantType(value))
	default:
		return refrigerantPointer(domain.Refrigerant410A)
	}
}

func voltagePointerOrDefault(value string) *domain.Voltage {
	parsed := domain.Voltage(value)
	switch parsed {
	case domain.Voltage220, domain.Voltage110, domain.VoltageBivolt:
		return &parsed
	default:
		fallback := domain.Voltage220
		return &fallback
	}
}

func parseFloatPointer(value string) *float64 {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) {
		return nil
	}
	return &number
}

func addMonthsISO(date string, months int, location *time.Location, fallback time.Time) string {
	base, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return fallback.Format("2006-01-02")
	}
	return domain.AddMonthsClamped(base, months).Format("2006-01-02")
}

func formatBrazilianNumber(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "12.000"
	}
	value = math.Round(value*1000) / 1000
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	parts := strings.SplitN(formatted, ".", 2)
	integer := parts[0]
	negative := strings.HasPrefix(integer, "-")
	if negative {
		integer = strings.TrimPrefix(integer, "-")
	}
	var grouped strings.Builder
	for i, r := range integer {
		if i > 0 && (len(integer)-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(r)
	}
	result := grouped.String()
	if negative {
		result = "-" + result
	}
	if len(parts) == 2 {
		result += "," + parts[1]
	}
	return result
}

func collapseJavaScriptWhitespace(value string) string {
	var result strings.Builder
	pendingSpace := false
	for _, r := range value {
		if unicode.IsSpace(r) || r == '\uFEFF' {
			pendingSpace = result.Len() > 0
			continue
		}
		if pendingSpace {
			result.WriteByte(' ')
			pendingSpace = false
		}
		result.WriteRune(r)
	}
	return result.String()
}
