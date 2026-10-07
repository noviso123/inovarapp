package supabase

import (
	"encoding/json"
	"math"
	"unicode/utf16"

	"inovarapp/core/domain"
)

type SupabaseBudget struct {
	ID             string  `json:"id"`
	Number         string  `json:"numero"`
	ClientID       string  `json:"cliente_id"`
	ServiceID      string  `json:"service_id,omitempty"`
	Date           string  `json:"data"`
	ValidUntil     string  `json:"validade,omitempty"`
	Description    string  `json:"descricao,omitempty"`
	ServiceType    string  `json:"tipo_servico,omitempty"`
	LaborValue     float64 `json:"valor_mao_obra"`
	MaterialsValue float64 `json:"valor_material"`
	TotalValue     float64 `json:"valor_total"`
	Conditions     string  `json:"condicoes,omitempty"`
	Status         string  `json:"status"`
}

type SupabaseBudgetWrite struct {
	ID             string  `json:"id,omitempty"`
	Number         string  `json:"numero"`
	ClientID       string  `json:"cliente_id"`
	Date           string  `json:"data"`
	ValidUntil     string  `json:"validade"`
	Description    string  `json:"descricao"`
	ServiceType    string  `json:"tipo_servico"`
	LaborValue     float64 `json:"valor_mao_obra"`
	MaterialsValue float64 `json:"valor_material"`
	TotalValue     float64 `json:"valor_total"`
	Conditions     string  `json:"condicoes"`
	Technician     string  `json:"responsavel_tecnico"`
	Status         string  `json:"status"`
}

type budgetMetadata struct {
	Number         string              `json:"numero"`
	ClientName     string              `json:"clientName"`
	ClientPhone    string              `json:"clientPhone"`
	ClientAddress  string              `json:"clientAddress"`
	ClientDocument string              `json:"clientDocument"`
	EquipmentName  string              `json:"equipmentName"`
	ApplianceDesc  string              `json:"applianceDesc"`
	Items          []domain.BudgetItem `json:"items"`
	Discount       float64             `json:"discount"`
	TotalValue     float64             `json:"totalValue"`
	ExecutionTime  string              `json:"executionTime"`
	WarrantyTerms  string              `json:"warrantyTerms"`
	Notes          string              `json:"notes"`
	Signature      *string             `json:"assinatura"`
	SignedAt       *string             `json:"assinatura_em"`
	Paid           bool                `json:"pago"`
	PaidAt         *string             `json:"pago_em"`
	AmountReceived *float64            `json:"valor_recebido"`
	CancelledAt    *string             `json:"cancelado_em"`
	ApplianceID    *string             `json:"applianceId"`
	ServiceID      *string             `json:"service_id,omitempty"`
}

// MapSupabaseBudgetToLocal preserves the legacy JSON metadata and database fallbacks.
func MapSupabaseBudgetToLocal(row SupabaseBudget) domain.BudgetEstimate {
	var meta budgetMetadata
	if row.Description != "" {
		_ = json.Unmarshal([]byte(row.Description), &meta)
	}

	items := make([]domain.BudgetItem, 0, len(meta.Items))
	items = append(items, meta.Items...)
	if len(items) == 0 && row.Description != "" {
		description := truncateUTF16(row.Description, 200)
		items = []domain.BudgetItem{{
			ID: "1", Description: description, Quantity: 1,
			UnitPrice: numberOrZeroFloat(row.TotalValue), TotalPrice: numberOrZeroFloat(row.TotalValue),
			Category: domain.BudgetItemService,
		}}
	}

	totalValue := numberOrZeroFloat(row.LaborValue) + numberOrZeroFloat(row.MaterialsValue)
	if totalValue == 0 {
		totalValue = numberOrZeroFloat(meta.TotalValue)
	}
	validUntil := row.ValidUntil
	if validUntil == "" {
		validUntil = row.Date
	}
	status := domain.BudgetPending
	if row.Status == "APROVADO" {
		status = domain.BudgetApproved
	} else if row.Status == "RECUSADO" {
		status = domain.BudgetDeclined
	} else if row.Status == "CANCELADO" {
		status = domain.BudgetCancelled
	}

	serviceID := nullableString(row.ServiceID)
	if serviceID == nil {
		serviceID = cloneString(meta.ServiceID)
	}
	return domain.BudgetEstimate{
		ID: row.ID, Number: firstNonEmpty(meta.Number, row.Number),
		ClientID: row.ClientID, ClientName: firstNonEmpty(meta.ClientName, "Cliente Inovar"),
		ClientPhone: meta.ClientPhone, ClientAddress: meta.ClientAddress,
		ClientDocument: meta.ClientDocument, EquipmentName: meta.EquipmentName,
		ApplianceDescription: firstNonEmpty(meta.ApplianceDesc, row.ServiceType),
		Date:                 row.Date, ValidUntil: validUntil, Items: items,
		TotalValue: totalValue, Discount: numberOrZeroFloat(meta.Discount),
		FinalValue: numberOrZeroFloat(row.TotalValue), PaymentConditions: firstNonEmpty(row.Conditions, "A combinar"),
		ExecutionTime: firstNonEmpty(meta.ExecutionTime, "A combinar"),
		WarrantyTerms: firstNonEmpty(meta.WarrantyTerms, "Não informado — parametrização não registrada neste orçamento"),
		Status:        status, CancelledAt: nonEmptyPointer(meta.CancelledAt), Notes: meta.Notes, Signature: nonEmptyPointer(meta.Signature),
		SignedAt: nonEmptyPointer(meta.SignedAt), Paid: boolPointer(meta.Paid),
		PaidAt: nonEmptyPointer(meta.PaidAt), AmountReceived: cloneFloat(meta.AmountReceived),
		ApplianceID: nonEmptyPointer(meta.ApplianceID), ServiceID: serviceID,
	}
}

// MapLocalBudgetToSupabase mirrors the legacy database write contract.
func MapLocalBudgetToSupabase(budget domain.BudgetEstimate, number, clientID, technician string) (SupabaseBudgetWrite, error) {
	var labor, materials float64
	for _, item := range budget.Items {
		amount := numberOrZeroFloat(item.TotalPrice)
		if item.Category == domain.BudgetItemService {
			labor += amount
		} else {
			materials += amount
		}
	}

	meta := budgetMetadata{
		Number: number, ClientName: budget.ClientName, ClientPhone: budget.ClientPhone,
		ClientAddress: budget.ClientAddress, ClientDocument: budget.ClientDocument,
		EquipmentName: budget.EquipmentName, ApplianceDesc: budget.ApplianceDescription,
		Items: budget.Items, Discount: budget.Discount, TotalValue: budget.TotalValue,
		ExecutionTime: budget.ExecutionTime, WarrantyTerms: budget.WarrantyTerms,
		Notes: budget.Notes, Signature: nonEmptyPointer(budget.Signature),
		SignedAt: nonEmptyPointer(budget.SignedAt), CancelledAt: nonEmptyPointer(budget.CancelledAt), Paid: budget.Paid != nil && *budget.Paid,
		PaidAt: nonEmptyPointer(budget.PaidAt), AmountReceived: cloneFloat(budget.AmountReceived),
		ApplianceID: nonEmptyPointer(budget.ApplianceID), ServiceID: nonEmptyPointer(budget.ServiceID),
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return SupabaseBudgetWrite{}, err
	}
	serviceType := "Serviço Inovar"
	if len(budget.Items) > 0 && budget.Items[0].Description != "" {
		serviceType = truncateUTF16(budget.Items[0].Description, 120)
	} else if budget.ApplianceDescription != "" {
		serviceType = truncateUTF16(budget.ApplianceDescription, 120)
	}
	return SupabaseBudgetWrite{
		ID:     budget.ID,
		Number: number, ClientID: clientID, Date: budget.Date, ValidUntil: budget.ValidUntil,
		Description: string(encoded), ServiceType: serviceType,
		LaborValue: labor, MaterialsValue: materials, TotalValue: budget.FinalValue,
		Conditions: budget.PaymentConditions, Technician: technician, Status: "ENVIADO",
	}, nil
}

func truncateUTF16(value string, maxUnits int) string {
	runes := []rune(value)
	units := 0
	end := 0
	for _, r := range runes {
		width := len(utf16.Encode([]rune{r}))
		if units+width > maxUnits {
			break
		}
		units += width
		end++
	}
	return string(runes[:end])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func numberOrZeroFloat(value float64) float64 {
	if math.IsNaN(value) || value == 0 {
		return 0
	}
	return value
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nonEmptyPointer(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	return cloneString(value)
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func boolPointer(value bool) *bool {
	copy := value
	return &copy
}
