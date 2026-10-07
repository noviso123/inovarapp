package domain

import (
	"regexp"
	"strconv"
	"strings"
)

// ServiceCard keeps the fixed catalog's copy and visual tokens together so each
// platform can render the same service without importing React-specific icons.
type ServiceCard struct {
	Type            ServiceType `json:"type"`
	Title           string      `json:"title"`
	Subtitle        string      `json:"subtitle"`
	Description     string      `json:"desc"`
	AverageTime     string      `json:"tempoMedio"`
	DefaultWarranty string      `json:"garantiaPadrao"`
	IncludedItems   []string    `json:"itensInclusos"`
	ReferencePrice  string      `json:"valorMedioRef"`
	Badge           string      `json:"badge"`
	Icon            string      `json:"icon"`
	AccentColor     string      `json:"accentColor"`
	GradientClass   string      `json:"gradientClass"`
	Color           string      `json:"color"`
}

type CatalogEntry struct {
	Key             string       `json:"key"`
	FixedType       ServiceType  `json:"fixo,omitempty"`
	Name            string       `json:"nome"`
	Price           float64      `json:"preco"`
	Edited          bool         `json:"editado,omitempty"`
	Description     string       `json:"desc,omitempty"`
	AverageTime     string       `json:"tempoMedio,omitempty"`
	DefaultWarranty string       `json:"garantiaPadrao,omitempty"`
	Badge           string       `json:"badge,omitempty"`
	Items           []string     `json:"itens,omitempty"`
	Card            *ServiceCard `json:"card,omitempty"`
}

var fixedServiceCards = []ServiceCard{
	{
		Type:        ServiceCleaning,
		Title:       "Limpeza de Ar Completa",
		Subtitle:    "Com bactericida hospitalar e lavagem de serpentina",
		Description: "Desmontagem da carenagem, limpeza com bolsa coletora e lava jato pressurizado, assepsia de serpentina, turbina e bandeja de dreno.",
		AverageTime: "1h20 min", DefaultWarranty: "90 dias",
		IncludedItems: []string{
			"Lavagem de filtros e carenagem plástica",
			"Desinfecção de serpentina com bactericida",
			"Limpeza profunda da turbina de ventilação",
			"Desobstrução do dreno e lavagem da bandeja",
			"Medição de salto térmico (ΔT) e corrente",
		},
		ReferencePrice: "R$ 280", Badge: "Mais Executado", Icon: "sparkles",
		AccentColor: "text-emerald-400", GradientClass: "from-emerald-950/40 via-slate-900 to-slate-900 border-emerald-500/30", Color: "emerald",
	},
	{
		Type:        ServiceInstallation,
		Title:       "Instalação Split / Inverter",
		Description: "Instalação em conformidade com as normas dos fabricantes, vácuo controlado abaixo de 500 microns e teste de estanqueidade.",
		Subtitle:    "Split Hi-Wall, Multi-Split e Inverter",
		AverageTime: "3h - 4h", DefaultWarranty: "180 a 365 dias",
		IncludedItems: []string{
			"Fixação com suporte nivelado e buchas apropriadas",
			"Tubulação de cobre com isolamento térmico blindado",
			"Vácuo profundo com vacuômetro digital (< 500 µ)",
			"Teste de estanqueidade e vazamento com N2",
			"Liberação controlada de fluido refrigerante",
		},
		ReferencePrice: "R$ 1.300", Badge: "Padrão Técnico", Icon: "zap",
		AccentColor: "text-sky-400", GradientClass: "from-sky-950/40 via-slate-900 to-slate-900 border-sky-500/30", Color: "sky",
	},
	{
		Type:        ServiceCorrective,
		Title:       "Conserto & Troca de Peças",
		Description: "Diagnóstico de falhas elétricas e mecânicas, substituição de componentes danificados com teste de carga e rendimento.",
		Subtitle:    "Capacitores, placas, ventiladores e sensores",
		AverageTime: "1h - 2h", DefaultWarranty: "90 dias",
		IncludedItems: []string{
			"Diagnóstico com capacímetro e osciloscópio/multímetro",
			"Substituição de capacitor de partida / ventilação",
			"Reparo ou troca de placa eletrônica / display",
			"Troca de sensor de temperatura e degelo",
			"Teste de funcionamento contínuo e desarme",
		},
		ReferencePrice: "R$ 250 - 625 + peças", Badge: "Diagnóstico Preciso", Icon: "wrench",
		AccentColor: "text-amber-400", GradientClass: "from-amber-950/40 via-slate-900 to-slate-900 border-amber-500/30", Color: "amber",
	},
	{
		Type:        ServiceRefrigerant,
		Title:       "Carga de Gás Refrigerante",
		Description: "Localização e correção de microvazamentos em flanges/soldas e reposição exata de fluido refrigerante por peso na balança digital.",
		Subtitle:    "R-410A, R-32, R-22 na balança de precisão",
		AverageTime: "1h30 min", DefaultWarranty: "90 dias",
		IncludedItems: []string{
			"Teste de pressurização com nitrogênio e detector",
			"Reaperto e refazimento de flanges com vazamento",
			"Vácuo para retirada de umidade do circuito",
			"Carga em fase líquida por balança (gramas)",
			"Medição de superaquecimento e sub-resfriamento",
		},
		ReferencePrice: "R$ 305 - 625", Badge: "Pesagem Exata", Icon: "flame",
		AccentColor: "text-purple-400", GradientClass: "from-purple-950/40 via-slate-900 to-slate-900 border-purple-500/30", Color: "purple",
	},
	{
		Type:        ServicePreventive,
		Title:       "Manutenção Preventiva / PMOC",
		Description: "Plano de Manutenção, Operação e Controle para empresas, consultórios e residências com laudo técnico e conformidade Anvisa.",
		Subtitle:    "Conformidade legal e máxima economia energética",
		AverageTime: "Mensal / Trimestral", DefaultWarranty: "Contrato ativo",
		IncludedItems: []string{
			"Limpeza de ar periódica programada",
			"Medição de consumo e corrente elétrica (A)",
			"Aferição de pressões e temperatura insuflada",
			"Emissão de Laudo Técnico e Termo de Garantia",
			"Prioridade de atendimento em chamados",
		},
		ReferencePrice: "Contratos / Sob Consulta", Badge: "Empresarial & Residencial", Icon: "shield-check",
		AccentColor: "text-blue-400", GradientClass: "from-blue-950/40 via-slate-900 to-slate-900 border-blue-500/30", Color: "blue",
	},
	{
		Type:        ServiceTechnicalAssessment,
		Title:       "Avaliação & Visita Técnica",
		Description: "Visita para dimensionamento de carga térmica (BTUs por m²), vistoria de instalações elétricas e laudo de orçamento.",
		Subtitle:    "Cálculo de carga térmica e vistoria",
		AverageTime: "45 min", DefaultWarranty: "Orçamento 15 dias",
		IncludedItems: []string{
			"Cálculo de incidência solar e metragem cúbica",
			"Inspeção do quadro elétrico e aterramento",
			"Avaliação das tubulações e pontos de dreno",
			"Proposta orçamentária detalhada",
		},
		ReferencePrice: "R$ 110 - 170 (abatível)", Badge: "Visita Técnica", Icon: "wind",
		AccentColor: "text-slate-300", GradientClass: "from-slate-800/40 via-slate-900 to-slate-900 border-slate-700", Color: "slate",
	},
}

var referencePricePattern = regexp.MustCompile(`([0-9.]+)`)

// ExtractReferencePrice mirrors the current catalog's first-number rule.
func ExtractReferencePrice(reference string) float64 {
	match := referencePricePattern.FindStringSubmatch(reference)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ".", ""), 64)
	if err != nil {
		return 0
	}
	return value
}

// BuildServiceCatalog combines the six fixed entries with profile edits and
// custom entries using the same key/order/name rules as the current client.
func BuildServiceCatalog(profile *TechnicianProfile) []CatalogEntry {
	removed := []string(nil)
	edited := []EditedFixedServiceType(nil)
	custom := []CustomServiceType(nil)
	if profile != nil {
		removed = profile.RemovedFixedServiceTypes
		edited = profile.EditedFixedServiceTypes
		custom = profile.CustomServiceTypes
	}

	entries := make([]CatalogEntry, 0, len(fixedServiceCards)+len(custom))
	for _, fixed := range fixedServiceCards {
		isRemoved := false
		for _, name := range removed {
			if NormalizeServiceName(name) == string(fixed.Type) {
				isRemoved = true
				break
			}
		}
		if isRemoved {
			continue
		}

		var fixedEdit *EditedFixedServiceType
		for i := range edited {
			if NormalizeServiceName(edited[i].Type) == string(fixed.Type) {
				fixedEdit = &edited[i]
				break
			}
		}
		card := cloneServiceCard(fixed)
		if fixedEdit == nil {
			entries = append(entries, CatalogEntry{
				Key: "fixo:" + string(fixed.Type), FixedType: fixed.Type,
				Name: string(fixed.Type), Price: ExtractReferencePrice(fixed.ReferencePrice), Card: &card,
			})
			continue
		}
		name := fixedEdit.Name
		if strings.TrimSpace(name) == "" {
			name = string(fixed.Type)
		}
		entries = append(entries, CatalogEntry{
			Key: "fixo:" + string(fixed.Type), FixedType: fixed.Type,
			Name: NormalizeServiceName(name), Price: fixedEdit.Price, Edited: true,
			Description: fixedEdit.Description, AverageTime: fixedEdit.AverageTime,
			DefaultWarranty: fixedEdit.DefaultWarranty, Badge: fixedEdit.Badge,
			Items: cloneStrings(fixedEdit.Items), Card: &card,
		})
	}

	for i, item := range custom {
		entries = append(entries, CatalogEntry{
			Key: "custom:" + strconv.Itoa(i), Name: NormalizeServiceName(item.Name),
			Price: item.Price, Description: item.Description, AverageTime: item.AverageTime,
			DefaultWarranty: item.DefaultWarranty, Badge: item.Badge, Items: cloneStrings(item.Items),
		})
	}
	return entries
}

func cloneServiceCard(card ServiceCard) ServiceCard {
	card.IncludedItems = cloneStrings(card.IncludedItems)
	return card
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
