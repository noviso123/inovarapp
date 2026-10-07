package pdf

import (
	"bytes"
	"embed"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"
	"inovarapp/core/domain"
)

//go:embed assets/INOVAR_HEADER_PDF.png assets/INOVAR_FOOTER_PDF.png assets/INOVAR_SIGNATURE_GABRIEL_PDF.png assets/DejaVuSansCondensed.ttf assets/DejaVuSansCondensed-Bold.ttf
var serviceOrderAssets embed.FS

type ServiceOrderData struct {
	Client    domain.Client
	Appliance domain.Appliance
	Record    domain.MaintenanceRecord
	Profile   domain.TechnicianProfile
}

func GenerateServiceOrder(data ServiceOrderData) ([]byte, error) {
	return BuildServiceOrderPDF(data.Client, data.Appliance, data.Record, data.Profile)
}

// BuildServiceOrderPDF reproduces the one-page A4 service receipt and warranty
// document used by the existing app, including its official brand artwork.
func BuildServiceOrderPDF(client domain.Client, appliance domain.Appliance, record domain.MaintenanceRecord, profile domain.TechnicianProfile) ([]byte, error) {
	header, err := serviceOrderAssets.ReadFile("assets/INOVAR_HEADER_PDF.png")
	if err != nil {
		return nil, err
	}
	footer, err := serviceOrderAssets.ReadFile("assets/INOVAR_FOOTER_PDF.png")
	if err != nil {
		return nil, err
	}
	signature, err := serviceOrderAssets.ReadFile("assets/INOVAR_SIGNATURE_GABRIEL_PDF.png")
	if err != nil {
		return nil, err
	}
	regular, err := serviceOrderAssets.ReadFile("assets/DejaVuSansCondensed.ttf")
	if err != nil {
		return nil, err
	}
	bold, err := serviceOrderAssets.ReadFile("assets/DejaVuSansCondensed-Bold.ttf")
	if err != nil {
		return nil, err
	}

	profile = withProfileDefaults(profile)
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(false, 8)
	pdf.AddUTF8FontFromBytes("DejaVu", "", regular)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", bold)
	pdf.AddPage()
	pdf.RegisterImageOptionsReader("inovar-header", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(header))
	pdf.RegisterImageOptionsReader("inovar-footer", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(footer))
	if pdf.Error() != nil {
		return nil, pdf.Error()
	}

	navy, ink, line := []int{9, 30, 78}, []int{35, 39, 48}, []int{130, 135, 145}
	y := 61.0
	pdf.SetTextColor(navy[0], navy[1], navy[2])
	pdf.SetFont("DejaVu", "B", 16)
	pdf.SetXY(10, y)
	pdf.CellFormat(190, 7, "ORDEM DE SERVIÇO E GARANTIA", "", 0, "C", false, 0, "")
	y += 10
	pdf.SetTextColor(ink[0], ink[1], ink[2])
	pdf.SetFont("DejaVu", "", 8.2)
	labelLine(pdf, "Data de conclusão:", formatDate(firstNonEmpty(record.CompletionDate, record.Date)), 17, y, 76)
	labelLine(pdf, "OS nº:", strings.ToUpper(lastChars(safe(record.ID), 8)), 130, y, 64)
	y += 8
	labelLine(pdf, "Cliente:", safe(client.Name), 17, y, 84)
	labelLine(pdf, "CNPJ:", safe(profile.CNPJ), 124, y, 70)
	y += 8
	address := joinNonEmpty(ptrString(client.Address), ptrString(client.Neighborhood), ptrString(client.City))
	if address == "" {
		address = "Não informado"
	}
	labelLine(pdf, "Endereço:", address, 17, y, 177)
	y += 6
	equipment := joinNonEmpty(appliance.Brand, ptrString(appliance.Model))
	if equipment == "" {
		equipment = "Não informado"
	}
	labelLine(pdf, "Equipamento:", fmt.Sprintf("%s — %s BTUs (%s)", equipment, safe(appliance.CapacityBTU), safe(appliance.Room)), 17, y, 177)
	y += 12

	sectionBar(pdf, "SERVIÇO REALIZADO", y, navy)
	y += 10
	pdf.SetDrawColor(line[0], line[1], line[2])
	pdf.SetFillColor(255, 255, 255)
	pdf.RoundedRect(16, y, 178, 56, 1.5, "1234", "FD")
	pdf.SetTextColor(ink[0], ink[1], ink[2])
	pdf.SetFont("DejaVu", "B", 8.5)
	serviceLines := pdf.SplitText(strings.ToUpper(safe(string(record.ServiceType))), 116)
	pdf.SetXY(22, y+6)
	pdf.MultiCell(116, 4.2, strings.Join(serviceLines, "\n"), "", "L", false)
	pdf.SetFont("DejaVu", "", 7.5)
	pdf.SetXY(145, y+6)
	pdf.CellFormat(43, 4, "Data: "+formatDate(firstNonEmpty(record.CompletionDate, record.Date)), "", 0, "R", false, 0, "")
	left, right := serviceChecklist(record)
	pdf.SetFont("DejaVu", "", 7)
	for index, text := range left {
		pdf.SetXY(22, y+18+float64(index)*8)
		pdf.MultiCell(84, 4, text, "", "L", false)
	}
	for index, text := range right {
		pdf.SetXY(110, y+18+float64(index)*8)
		pdf.MultiCell(78, 4, text, "", "L", false)
	}
	y += 64

	pdf.SetFillColor(239, 243, 249)
	pdf.SetDrawColor(line[0], line[1], line[2])
	pdf.RoundedRect(16, y, 178, 43, 1.5, "1234", "FD")
	pdf.SetTextColor(navy[0], navy[1], navy[2])
	pdf.SetFont("DejaVu", "B", 8.5)
	pdf.SetXY(22, y+5)
	pdf.CellFormat(166, 4, "VALORES E GARANTIA", "", 0, "L", false, 0, "")
	pdf.SetTextColor(ink[0], ink[1], ink[2])
	pdf.SetFont("DejaVu", "", 7.5)
	labor := record.Price
	if record.LaborPrice != nil {
		labor = *record.LaborPrice
	} else if record.PartsPrice != nil {
		labor = record.Price - *record.PartsPrice
	}
	parts := 0.0
	if record.PartsPrice != nil {
		parts = *record.PartsPrice
	}
	pdf.SetXY(22, y+13)
	pdf.CellFormat(80, 4, "Mão de obra: "+formatMoney(labor), "", 0, "L", false, 0, "")
	materials := serviceOrderMaterials(parts, record.PartsUsed)
	pdf.SetXY(105, y+13)
	pdf.MultiCell(83, 4, materials, "", "L", false)
	pdf.SetFont("DejaVu", "B", 7.5)
	pdf.SetXY(22, y+22)
	pdf.CellFormat(166, 4, fmt.Sprintf("Valor total: %s — %s", formatMoney(record.Price), safe(string(record.PaymentMethod))), "", 0, "L", false, 0, "")
	pdf.SetFont("DejaVu", "", 7.5)
	pdf.SetXY(105, y+22)
	pdf.CellFormat(83, 4, fmt.Sprintf("PIX: %s (%s)", safe(profile.PIXKey), strings.ToUpper(safe(string(profile.PIXType)))), "", 0, "L", false, 0, "")
	warrantyDays := record.WarrantyDays
	if warrantyDays == 0 {
		warrantyDays = profile.DefaultWarrantyDays
	}
	warrantyDate := warrantyExpiry(firstNonEmpty(record.CompletionDate, record.Date), warrantyDays)
	pdf.SetTextColor(16, 120, 80)
	pdf.SetXY(22, y+31)
	pdf.CellFormat(83, 4, fmt.Sprintf("Garantia: %d dias — válida até %s", warrantyDays, warrantyDate), "", 0, "L", false, 0, "")
	if record.ReturnDate != "" {
		pdf.SetTextColor(80, 90, 105)
		pdf.SetFont("DejaVu", "", 7)
		pdf.SetXY(105, y+31)
		pdf.CellFormat(83, 4, "Próxima revisão: "+formatDate(record.ReturnDate), "", 0, "L", false, 0, "")
	}
	y += 52

	addTechnicianSignature(pdf, signature, y)
	pdf.SetDrawColor(line[0], line[1], line[2])
	pdf.Line(25, y+12, 85, y+12)
	pdf.Line(125, y+12, 185, y+12)
	pdf.SetTextColor(ink[0], ink[1], ink[2])
	pdf.SetFont("DejaVu", "", 7)
	pdf.SetXY(25, y+13)
	pdf.CellFormat(60, 4, "Assinatura do técnico", "", 0, "C", false, 0, "")
	pdf.SetXY(125, y+13)
	pdf.CellFormat(60, 4, "Assinatura do cliente", "", 0, "C", false, 0, "")
	pdf.SetFont("DejaVu", "", 6.5)
	pdf.SetXY(25, y+18)
	pdf.CellFormat(60, 4, "Gabriel Nascimento", "", 0, "C", false, 0, "")
	pdf.SetXY(125, y+18)
	pdf.CellFormat(60, 4, safe(client.Name), "", 0, "C", false, 0, "")
	pdf.RegisterImageOptionsReader("inovar-footer", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(footer))
	pdf.ImageOptions("inovar-footer", 8, 254, 194, 35, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	// Render the masthead after body sections so the full-width brand artwork
	// remains visible in the generated one-page document.
	pdf.ImageOptions("inovar-header", 0, 0, 210, 50.2, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	if pdf.Error() != nil {
		return nil, pdf.Error()
	}
	var result bytes.Buffer
	if err := pdf.Output(&result); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func serviceOrderMaterials(parts float64, partsUsed string) string {
	if parts <= 0 {
		return "Materiais/insumos: Inclusos"
	}
	line := "Peças/materiais: " + formatMoney(parts)
	if partsUsed != "" {
		line += " (" + safe(partsUsed) + ")"
	}
	return line
}

func withProfileDefaults(profile domain.TechnicianProfile) domain.TechnicianProfile {
	if profile.DefaultWarrantyDays == 0 {
		profile.DefaultWarrantyDays = 90
	}
	if profile.PIXKey == "" {
		profile.PIXKey = "gabrielnascimento458@gmail.com"
	}
	if profile.PIXType == "" {
		profile.PIXType = domain.PIXEmail
	}
	return profile
}

func sectionBar(pdf *gofpdf.Fpdf, label string, y float64, color []int) {
	pdf.SetFillColor(color[0], color[1], color[2])
	pdf.RoundedRect(16, y, 178, 10, 1.5, "1234", "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("DejaVu", "B", 8.7)
	pdf.SetXY(22, y+2.5)
	pdf.CellFormat(166, 5, label, "", 0, "L", false, 0, "")
}

func labelLine(pdf *gofpdf.Fpdf, label, value string, x, y, width float64) {
	pdf.SetFont("DejaVu", "", 8.2)
	labelWidth := pdf.GetStringWidth(label) + 2
	pdf.Text(x, y, label)
	pdf.SetDrawColor(130, 135, 145)
	pdf.Line(x+labelWidth, y+1, x+width, y+1)
	pdf.SetXY(x+labelWidth+2, y-3)
	pdf.CellFormat(width-labelWidth-3, 5, safe(value), "", 0, "L", false, 0, "")
}

func serviceChecklist(record domain.MaintenanceRecord) ([]string, []string) {
	checklist := record.Checklist
	if checklist == nil {
		checklist = &domain.ChecklistData{}
	}
	mark := func(value *bool) string {
		if value == nil || *value {
			return "X"
		}
		return " "
	}
	switch record.ServiceType {
	case domain.ServiceInstallation:
		return []string{
			fmt.Sprintf("[%s] Fixação e nivelamento", mark(checklist.BracketLeveled)),
			fmt.Sprintf("[%s] Vácuo: %s", conditionalMark(checklist.VacuumMicrons != ""), safe(checklist.VacuumMicrons)),
			fmt.Sprintf("[%s] Teste de estanqueidade", mark(checklist.NitrogenTest)),
			fmt.Sprintf("[%s] Válvulas liberadas", mark(checklist.ValvesReleased)),
		}, []string{"Salto térmico: " + safe(checklist.ThermalDeltaT), "Corrente: " + safe(checklist.CurrentAmps), "Dreno testado", "Tubulação isolada"}
	case domain.ServiceCorrective:
		return []string{"Diagnóstico: " + safe(checklist.TechnicalDiagnosis), "Peças: " + safe(firstNonEmpty(record.PartsUsed, checklist.PartsReplaced)), "Componente: " + safe(checklist.CapacitorTested), "Teste elétrico realizado"}, []string{"Corrente: " + safe(checklist.CurrentAmps), "Pressão: " + safe(checklist.GasPressurePSI), "Salto térmico: " + safe(checklist.ThermalDeltaT), "Teste operacional realizado"}
	case domain.ServiceRefrigerant:
		return []string{"Fluido adicionado: " + safe(checklist.GasAddedGrams), "Teste de vazamento realizado", "Vácuo prévio realizado", "Carga por balança"}, []string{"Pressão: " + safe(checklist.GasPressurePSI), "Salto térmico: " + safe(checklist.ThermalDeltaT), "Corrente: " + safe(checklist.CurrentAmps), "Sistema testado"}
	default:
		return []string{
			fmt.Sprintf("[%s] Filtros lavados", mark(checklist.FiltersWashed)),
			fmt.Sprintf("[%s] Serpentina limpa", mark(checklist.CoilSanitized)),
			fmt.Sprintf("[%s] Turbina limpa", mark(checklist.TurbineCleaned)),
			fmt.Sprintf("[%s] Condensadora lavada", mark(checklist.CondenserWashed)),
		}, []string{
			fmt.Sprintf("[%s] Dreno desobstruído", mark(checklist.DrainUnclogged)),
			fmt.Sprintf("[%s] Bactericida aplicado", mark(checklist.BactericideApplied)),
			"Salto térmico: " + safe(checklist.ThermalDeltaT),
			"Pressão/corrente: " + safe(checklist.GasPressurePSI) + " / " + safe(checklist.CurrentAmps),
		}
	}
}

func addTechnicianSignature(pdf *gofpdf.Fpdf, signature []byte, y float64) {
	pdf.RegisterImageOptionsReader("inovar-signature", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(signature))
	if pdf.Error() != nil {
		return
	}
	info := pdf.GetImageInfo("inovar-signature")
	if info == nil || info.Width() == 0 || info.Height() == 0 {
		return
	}
	width, height := 62.0, 16.0
	ratio := info.Width() / info.Height()
	actualHeight := width / ratio
	if actualHeight > height {
		actualHeight = height
		width = actualHeight * ratio
	}
	pdf.ImageOptions("inovar-signature", 55-width/2, y-3+(height-actualHeight)/2, width, actualHeight, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
}

func conditionalMark(condition bool) string {
	if condition {
		return "X"
	}
	return " "
}

func formatMoney(value float64) string {
	value = math.Round(value*100) / 100
	formatted := strconv.FormatFloat(value, 'f', 2, 64)
	parts := strings.SplitN(formatted, ".", 2)
	whole := parts[0]
	for index := len(whole) - 3; index > 0; index -= 3 {
		whole = whole[:index] + "." + whole[index:]
	}
	return "R$ " + whole + "," + parts[1]
}

func warrantyExpiry(date string, days int) string {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		parsed = time.Now()
	}
	return parsed.AddDate(0, 0, days).Format("02/01/2006")
}

func formatDate(value string) string {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value[:min(len(value), 10)]))
	if err != nil {
		return "Não informado"
	}
	return parsed.Format("02/01/2006")
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func joinNonEmpty(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ", ")
}

func safe(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")
	if strings.TrimSpace(value) == "" {
		return "Não informado"
	}
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func lastChars(value string, count int) string {
	if len(value) <= count {
		return value
	}
	return value[len(value)-count:]
}
