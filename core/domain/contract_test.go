package domain

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// These mappings are the shared data contract between the Go core and the
// still-supported local backup/client schema. Adding or removing a field in
// either source now requires an explicit migration decision.
func TestCoreModelsMatchTypeScriptDataContracts(t *testing.T) {
	source, err := os.ReadFile("../../src/types/index.ts")
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(source)
	cases := []struct {
		goType, tsType string
		value          any
	}{
		{"Appliance", "Appliance", Appliance{}},
		{"ChecklistData", "ChecklistData", ChecklistData{}},
		{"MaintenanceRecord", "MaintenanceRecord", MaintenanceRecord{}},
		{"Client", "Client", Client{}},
		{"TechnicianProfile", "TechnicianProfile", TechnicianProfile{}},
		{"DadosTipoServico", "DadosTipoServico", DadosTipoServico{}},
		{"CustomServiceType", "TipoServicoCustom", CustomServiceType{}},
		{"EditedFixedServiceType", "TipoFixoEditado", EditedFixedServiceType{}},
		{"BudgetItem", "BudgetItem", BudgetItem{}},
		{"BudgetEstimate", "BudgetEstimate", BudgetEstimate{}},
	}
	for _, tc := range cases {
		t.Run(tc.goType, func(t *testing.T) {
			goFields := jsonFieldNames(reflect.TypeOf(tc.value))
			tsFields, ok := tsInterfaceFields(legacy, tc.tsType)
			if !ok {
				t.Fatalf("TypeScript interface %s not found", tc.tsType)
			}
			if diff := fieldDifference(goFields, tsFields); len(diff) > 0 {
				t.Fatalf("Go %s and TypeScript %s differ: %s", tc.goType, tc.tsType, strings.Join(diff, "; "))
			}
		})
	}
}

func TestCoreEnumsMatchTypeScriptLiteralUnions(t *testing.T) {
	source, err := os.ReadFile("../../src/types/index.ts")
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(source)
	cases := []struct {
		name   string
		values []string
	}{
		{"ReturnStatus", []string{string(ReturnOverdue), string(ReturnThisWeek), string(ReturnSoon), string(ReturnOnTime), string(ReturnNoHistory)}},
		{"ServiceType", []string{string(ServiceCleaning), string(ServiceInstallation), string(ServiceCorrective), string(ServiceRefrigerant), string(ServicePreventive), string(ServiceTechnicalAssessment), string(ServiceOther)}},
	}
	for _, tc := range cases {
		want, ok := tsLiteralUnion(legacy, tc.name)
		if !ok {
			t.Fatalf("TypeScript union %s not found", tc.name)
		}
		if diff := fieldDifference(tc.values, want); len(diff) > 0 {
			t.Errorf("Go enum %s and TypeScript union differ: %s", tc.name, strings.Join(diff, "; "))
		}
	}
}

func TestInlineModelEnumsMatchGoCore(t *testing.T) {
	source, err := os.ReadFile("../../src/types/index.ts")
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(source)
	cases := []struct {
		model, field string
		values       []string
	}{
		{"Appliance", "type", []string{string(ApplianceSplit), string(ApplianceInverter), string(ApplianceCassette), string(ApplianceFloorCeiling), string(ApplianceWindow), string(ApplianceMultiSplit), string(AppliancePortable)}},
		{"Appliance", "gasType", []string{string(Refrigerant410A), string(Refrigerant32), string(Refrigerant22), string(RefrigerantOther)}},
		{"Appliance", "voltage", []string{string(Voltage220), string(Voltage110), string(VoltageBivolt)}},
		{"MaintenanceRecord", "paymentMethod", []string{string(PaymentPIX), string(PaymentCreditCard), string(PaymentDebitCard), string(PaymentCash), string(PaymentInvoiced)}},
		{"MaintenanceRecord", "status", []string{string(MaintenanceCompleted), string(MaintenanceScheduled), string(MaintenanceInProgress), string(MaintenanceCancelled)}},
		{"TechnicianProfile", "pixType", []string{string(PIXCPF), string(PIXCNPJ), string(PIXEmail), string(PIXPhone), string(PIXRand)}},
		{"BudgetItem", "category", []string{string(BudgetItemService), string(BudgetItemPart), string(BudgetItemMaterial)}},
		{"BudgetEstimate", "status", []string{string(BudgetPending), string(BudgetApproved), string(BudgetDeclined), string(BudgetCancelled)}},
	}
	for _, tc := range cases {
		want, ok := tsFieldLiteralUnion(legacy, tc.model, tc.field)
		if !ok {
			t.Errorf("TypeScript field %s.%s not found or is not a literal union", tc.model, tc.field)
			continue
		}
		if diff := fieldDifference(tc.values, want); len(diff) > 0 {
			t.Errorf("Go values for %s.%s and TypeScript differ: %s", tc.model, tc.field, strings.Join(diff, "; "))
		}
	}
}

var (
	tsInterfaceStart = regexp.MustCompile(`(?m)^export interface ([A-Za-z0-9_]+)(?: extends ([A-Za-z0-9_]+))? \{`)
	tsField          = regexp.MustCompile(`^\s*([A-Za-z_$][A-Za-z0-9_$]*)\??\s*:`)
	tsFieldType      = regexp.MustCompile(`^\s*([A-Za-z_$][A-Za-z0-9_$]*)\??\s*:\s*([^;]+)`)
	tsUnionStart     = regexp.MustCompile(`(?s)export type ([A-Za-z0-9_]+)\s*=([^;]+);`)
	tsLiteral        = regexp.MustCompile(`'([^']*)'`)
)

func tsInterfaceFields(source, name string) ([]string, bool) {
	for _, match := range tsInterfaceStart.FindAllStringSubmatchIndex(source, -1) {
		interfaceName := source[match[2]:match[3]]
		if interfaceName != name {
			continue
		}
		extends := ""
		if match[4] >= 0 {
			extends = source[match[4]:match[5]]
		}
		open := match[1] - 1
		close := balancedBraceEnd(source, open)
		if close < 0 {
			return nil, false
		}
		fields := []string{}
		if extends != "" {
			parent, ok := tsInterfaceFields(source, extends)
			if !ok {
				return nil, false
			}
			fields = append(fields, parent...)
		}
		for _, line := range strings.Split(source[open+1:close], "\n") {
			if field := tsField.FindStringSubmatch(line); len(field) == 2 {
				fields = append(fields, field[1])
			}
		}
		return uniqueSorted(fields), true
	}
	return nil, false
}

func balancedBraceEnd(source string, open int) int {
	depth := 0
	for i := open; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func jsonFieldNames(value reflect.Type) []string {
	fields := []string{}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.Anonymous {
			fields = append(fields, jsonFieldNames(field.Type)...)
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "-" {
			fields = append(fields, name)
		}
	}
	return uniqueSorted(fields)
}

func tsLiteralUnion(source, name string) ([]string, bool) {
	for _, match := range tsUnionStart.FindAllStringSubmatch(source, -1) {
		if match[1] != name {
			continue
		}
		values := []string{}
		for _, literal := range tsLiteral.FindAllStringSubmatch(match[2], -1) {
			values = append(values, literal[1])
		}
		return uniqueSorted(values), true
	}
	return nil, false
}

func tsFieldLiteralUnion(source, interfaceName, fieldName string) ([]string, bool) {
	open, close, ok := tsInterfaceBounds(source, interfaceName)
	if !ok {
		return nil, false
	}
	for _, line := range strings.Split(source[open+1:close], "\n") {
		match := tsFieldType.FindStringSubmatch(line)
		if len(match) != 3 || match[1] != fieldName {
			continue
		}
		values := []string{}
		for _, literal := range tsLiteral.FindAllStringSubmatch(match[2], -1) {
			values = append(values, literal[1])
		}
		return uniqueSorted(values), len(values) > 0
	}
	return nil, false
}

func tsInterfaceBounds(source, name string) (int, int, bool) {
	for _, match := range tsInterfaceStart.FindAllStringSubmatchIndex(source, -1) {
		if source[match[2]:match[3]] != name {
			continue
		}
		open := match[1] - 1
		close := balancedBraceEnd(source, open)
		return open, close, close >= 0
	}
	return 0, 0, false
}

func fieldDifference(goFields, tsFields []string) []string {
	goSet, tsSet := map[string]bool{}, map[string]bool{}
	for _, field := range goFields {
		goSet[field] = true
	}
	for _, field := range tsFields {
		tsSet[field] = true
	}
	diff := []string{}
	for field := range goSet {
		if !tsSet[field] {
			diff = append(diff, "Go-only "+field)
		}
	}
	for field := range tsSet {
		if !goSet[field] {
			diff = append(diff, "TypeScript-only "+field)
		}
	}
	sort.Strings(diff)
	return diff
}

func uniqueSorted(values []string) []string {
	set := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !set[value] {
			set[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
