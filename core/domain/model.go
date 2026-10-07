package domain

// Os nomes JSON preservam os contratos camelCase consumidos pelo cliente atual.
type ReturnStatus string

const (
	ReturnOverdue   ReturnStatus = "atrasado"
	ReturnThisWeek  ReturnStatus = "esta_semana"
	ReturnSoon      ReturnStatus = "em_breve"
	ReturnOnTime    ReturnStatus = "em_dia"
	ReturnNoHistory ReturnStatus = "sem_historico"
)

type Role string

const (
	RoleCustomer   Role = "CLIENTE"
	RoleTechnician Role = "TECNICO"
	RoleAdmin      Role = "ADMIN"
)

type ServiceType string

const (
	ServiceCleaning            ServiceType = "Limpeza de Ar"
	ServiceInstallation        ServiceType = "Instalação"
	ServiceCorrective          ServiceType = "Manutenção Corretiva"
	ServiceRefrigerant         ServiceType = "Recarga de Gás"
	ServicePreventive          ServiceType = "Manutenção Preventiva"
	ServiceTechnicalAssessment ServiceType = "Avaliação Técnica"
	ServiceOther               ServiceType = "Outro"
)

type ApplianceType string

const (
	ApplianceSplit        ApplianceType = "Split Hi-Wall"
	ApplianceInverter     ApplianceType = "Inverter"
	ApplianceCassette     ApplianceType = "Cassete"
	ApplianceFloorCeiling ApplianceType = "Piso Teto"
	ApplianceWindow       ApplianceType = "Janela"
	ApplianceMultiSplit   ApplianceType = "Multi Split"
	AppliancePortable     ApplianceType = "Portátil"
)

type RefrigerantType string

const (
	Refrigerant410A  RefrigerantType = "R-410A"
	Refrigerant32    RefrigerantType = "R-32"
	Refrigerant22    RefrigerantType = "R-22"
	RefrigerantOther RefrigerantType = "Outro"
)

type Voltage string

const (
	Voltage220    Voltage = "220V"
	Voltage110    Voltage = "110V"
	VoltageBivolt Voltage = "Bivolt"
)

type PaymentMethod string

const (
	PaymentPIX        PaymentMethod = "PIX"
	PaymentCreditCard PaymentMethod = "Cartão Crédito"
	PaymentDebitCard  PaymentMethod = "Cartão Débito"
	PaymentCash       PaymentMethod = "Dinheiro"
	PaymentInvoiced   PaymentMethod = "A Faturar"
)

type PIXKeyType string

const (
	PIXCPF   PIXKeyType = "cpf"
	PIXCNPJ  PIXKeyType = "cnpj"
	PIXEmail PIXKeyType = "email"
	PIXPhone PIXKeyType = "telefone"
	PIXRand  PIXKeyType = "aleatoria"
)

type MaintenanceStatus string

const (
	MaintenanceCompleted  MaintenanceStatus = "concluido"
	MaintenanceScheduled  MaintenanceStatus = "agendado"
	MaintenanceInProgress MaintenanceStatus = "em_andamento"
	MaintenanceCancelled  MaintenanceStatus = "cancelado"
)

type BudgetStatus string

const (
	BudgetPending   BudgetStatus = "pendente"
	BudgetApproved  BudgetStatus = "aprovado"
	BudgetDeclined  BudgetStatus = "recusado"
	BudgetCancelled BudgetStatus = "cancelado"
)

type Appliance struct {
	ID               string           `json:"id"`
	ClientID         string           `json:"clientId"`
	Brand            string           `json:"brand"`
	Model            *string          `json:"model,omitempty"`
	Type             ApplianceType    `json:"type"`
	CapacityBTU      string           `json:"capacityBtu"`
	Room             string           `json:"room"`
	SerialNumber     string           `json:"serialNumber,omitempty"`
	InstallationSite string           `json:"installationSite,omitempty"`
	GasType          *RefrigerantType `json:"gasType,omitempty"`
	Voltage          *Voltage         `json:"voltage,omitempty"`
	InstallDate      *string          `json:"installDate,omitempty"`
	Notes            *string          `json:"notes,omitempty"`
}

// Campos opcionais permanecem ponteiros para distinguir false/zero de ausente.
type ChecklistData struct {
	FiltersWashed      *bool  `json:"filtrosLavados,omitempty"`
	CoilSanitized      *bool  `json:"serpentinaHigienizada,omitempty"`
	TurbineCleaned     *bool  `json:"turbinaLimpa,omitempty"`
	DrainUnclogged     *bool  `json:"drenoDesobstruido,omitempty"`
	TraySanitized      *bool  `json:"bandejaSanitizada,omitempty"`
	CondenserWashed    *bool  `json:"condensadoraLavada,omitempty"`
	BactericideApplied *bool  `json:"aplicacaoBactericida,omitempty"`
	ElectricalTest     *bool  `json:"testeEletricoCorrente,omitempty"`
	CurrentAmps        string `json:"correnteAmperes,omitempty"`
	GasPressurePSI     string `json:"pressaoGasPSI,omitempty"`
	ThermalDeltaT      string `json:"saltoTermicoDeltaT,omitempty"`
	SupplyTemperature  string `json:"temperaturaInsuflamento,omitempty"`
	ReturnTemperature  string `json:"temperaturaRetorno,omitempty"`
	BracketLeveled     *bool  `json:"suporteNivelado,omitempty"`
	VacuumMicrons      string `json:"vacuoMicrons,omitempty"`
	NitrogenTest       *bool  `json:"testeNitrogenio,omitempty"`
	ValvesReleased     *bool  `json:"valvulasLiberadas,omitempty"`
	Superheat          string `json:"superaquecimentoUtil,omitempty"`
	CapacitorTested    string `json:"capacitorTestado,omitempty"`
	GasAddedGrams      string `json:"gasAdicionadoGramas,omitempty"`
	PartsReplaced      string `json:"pecasSubstituidas,omitempty"`
	TechnicalDiagnosis string `json:"diagnosticoTecnico,omitempty"`
}

type Attachment struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
	Type string `json:"type,omitempty"`
	Size *int64 `json:"size,omitempty"`
}

type MaintenanceRecord struct {
	ID                 string            `json:"id"`
	ClientID           string            `json:"clientId"`
	ApplianceID        string            `json:"applianceId"`
	Date               string            `json:"date"`
	ScheduledDate      string            `json:"scheduledDate,omitempty"`
	ScheduledTime      string            `json:"scheduledTime,omitempty"`
	CompletionDate     string            `json:"completionDate,omitempty"`
	CompletedAt        NullableString    `json:"completedAt,omitzero"`
	ReturnDate         string            `json:"returnDate"`
	ServiceType        ServiceType       `json:"serviceType"`
	Price              float64           `json:"price"`
	LaborPrice         *float64          `json:"laborPrice,omitempty"`
	PartsPrice         *float64          `json:"partsPrice,omitempty"`
	PartsUsed          string            `json:"partsUsed,omitempty"`
	PaymentMethod      PaymentMethod     `json:"paymentMethod"`
	WarrantyDays       int               `json:"warrantyDays"`
	Notes              *string           `json:"notes,omitempty"`
	Checklist          *ChecklistData    `json:"checklist,omitempty"`
	StartedAt          NullableString    `json:"startedAt,omitzero"`
	CancelledAt        NullableString    `json:"cancelledAt,omitzero"`
	CancellationReason string            `json:"cancellationReason,omitempty"`
	Status             MaintenanceStatus `json:"status"`
	ContactedAt        string            `json:"contactedAt,omitempty"`
	Attachments        []Attachment      `json:"attachments,omitempty"`
}

type Client struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Phone        string      `json:"phone"`
	Document     *string     `json:"document,omitempty"`
	Address      *string     `json:"address,omitempty"`
	Neighborhood *string     `json:"neighborhood,omitempty"`
	City         *string     `json:"city,omitempty"`
	Notes        *string     `json:"notes,omitempty"`
	CreatedAt    string      `json:"createdAt"`
	Appliances   []Appliance `json:"appliances"`
}

type DadosTipoServico struct {
	Name            string   `json:"nome"`
	Price           float64  `json:"preco"`
	Description     string   `json:"desc,omitempty"`
	AverageTime     string   `json:"tempoMedio,omitempty"`
	DefaultWarranty string   `json:"garantiaPadrao,omitempty"`
	Badge           string   `json:"badge,omitempty"`
	Items           []string `json:"itens,omitempty"`
}

type CustomServiceType DadosTipoServico

type EditedFixedServiceType struct {
	Type string `json:"tipo"`
	DadosTipoServico
}

type TechnicianProfile struct {
	Name                     string                   `json:"name"`
	BusinessName             string                   `json:"businessName"`
	Phone                    string                   `json:"phone"`
	PIXKey                   string                   `json:"pixKey"`
	PIXType                  PIXKeyType               `json:"pixType"`
	DefaultReturnMonths      int                      `json:"defaultReturnMonths"`
	DefaultWarrantyDays      int                      `json:"defaultWarrantyDays"`
	DefaultPrice             float64                  `json:"defaultPrice"`
	CNPJ                     string                   `json:"cnpj,omitempty"`
	Address                  string                   `json:"address,omitempty"`
	Signature                *string                  `json:"assinatura,omitempty"`
	CustomServiceTypes       []CustomServiceType      `json:"tiposServicosCustom,omitempty"`
	WhatsAppMessages         map[string]string        `json:"mensagensWhats,omitempty"`
	ReminderIntervalDays     int                      `json:"lembrete_intervalo_dias,omitempty"`
	RemovedFixedServiceTypes []string                 `json:"tiposFixosRemovidos,omitempty"`
	EditedFixedServiceTypes  []EditedFixedServiceType `json:"tiposFixosEditados,omitempty"`
	CalendarToken            string                   `json:"calendario_token,omitempty"`
}

type BudgetItem struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Quantity    float64            `json:"quantity"`
	UnitPrice   float64            `json:"unitPrice"`
	TotalPrice  float64            `json:"totalPrice"`
	Category    BudgetItemCategory `json:"category"`
}

type BudgetItemCategory string

const (
	BudgetItemService  BudgetItemCategory = "servico"
	BudgetItemPart     BudgetItemCategory = "peca"
	BudgetItemMaterial BudgetItemCategory = "material"
)

type BudgetEstimate struct {
	ID                   string       `json:"id"`
	Number               string       `json:"numero,omitempty"`
	ClientID             string       `json:"clientId"`
	ClientName           string       `json:"clientName"`
	ClientPhone          string       `json:"clientPhone"`
	ClientAddress        string       `json:"clientAddress,omitempty"`
	ClientDocument       string       `json:"clientDocument,omitempty"`
	EquipmentName        string       `json:"equipmentName,omitempty"`
	ApplianceDescription string       `json:"applianceDesc,omitempty"`
	Date                 string       `json:"date"`
	ValidUntil           string       `json:"validUntil"`
	Items                []BudgetItem `json:"items"`
	TotalValue           float64      `json:"totalValue"`
	Discount             float64      `json:"discount"`
	FinalValue           float64      `json:"finalValue"`
	PaymentConditions    string       `json:"paymentConditions"`
	ExecutionTime        string       `json:"executionTime"`
	WarrantyTerms        string       `json:"warrantyTerms"`
	Status               BudgetStatus `json:"status"`
	CancelledAt          *string      `json:"canceladoEm,omitempty"`
	Notes                string       `json:"notes,omitempty"`
	Signature            *string      `json:"assinatura,omitempty"`
	SignedAt             *string      `json:"assinaturaEm,omitempty"`
	Paid                 *bool        `json:"pago,omitempty"`
	PaidAt               *string      `json:"pagoEm,omitempty"`
	AmountReceived       *float64     `json:"valorRecebido,omitempty"`
	ApplianceID          *string      `json:"applianceId,omitempty"`
	ServiceID            *string      `json:"serviceId,omitempty"`
}
