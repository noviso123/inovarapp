package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"
	"inovarapp/core/domain"
)

// BuildBudgetPDF renders the branded commercial estimate used by the legacy
// client, keeping all text as UTF-8 and calculating pagination in Go.
func BuildBudgetPDF(budget domain.BudgetEstimate, profile domain.TechnicianProfile) ([]byte, error) {
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
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetMargins(8, 8, 8)
	doc.SetAutoPageBreak(false, 8)
	doc.AddUTF8FontFromBytes("DejaVu", "", regular)
	doc.AddUTF8FontFromBytes("DejaVu", "B", bold)
	doc.AddPage()
	doc.RegisterImageOptionsReader("budget-header", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(header))
	doc.RegisterImageOptionsReader("budget-footer", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(footer))
	doc.RegisterImageOptionsReader("budget-signature", gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(signature))
	if doc.Error() != nil {
		return nil, doc.Error()
	}

	navy, ink := []int{9, 30, 78}, []int{35, 39, 48}
	pageHeader := func() {
		doc.ImageOptions("budget-header", 0, 0, 210, 50.2, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	}
	pageHeader()
	y := 61.0
	doc.SetTextColor(navy[0], navy[1], navy[2])
	doc.SetFont("DejaVu", "B", 16)
	doc.SetXY(10, y)
	doc.CellFormat(190, 7, "ORÇAMENTO DE MANUTENÇÃO", "", 0, "C", false, 0, "")
	y += 10
	doc.SetTextColor(ink[0], ink[1], ink[2])
	doc.SetFont("DejaVu", "", 8)
	labelLine(doc, "Data:", formatDate(budget.Date), 17, y, 92)
	number := budget.Number
	if number == "" {
		number = budget.ID
	}
	labelLine(doc, "Orçamento nº:", number, 124, y, 70)
	y += 8
	labelLine(doc, "Cliente:", budget.ClientName, 17, y, 92)
	labelLine(doc, "CNPJ:", profile.CNPJ, 124, y, 70)
	y += 8
	address := budget.ClientAddress
	if address == "" {
		address = "Não informado"
	}
	labelLine(doc, "Endereço:", address, 17, y, 177)
	y += 8
	if strings.TrimSpace(budget.ClientPhone) != "" {
		labelLine(doc, "Telefone:", budget.ClientPhone, 17, y, 92)
		y += 8
	}
	if strings.TrimSpace(budget.EquipmentName) != "" {
		labelLine(doc, "Equipamento:", budget.EquipmentName, 17, y, 177)
		y += 8
	}

	doc.SetFillColor(navy[0], navy[1], navy[2])
	doc.RoundedRect(16, y, 178, 10, 1.5, "1234", "F")
	doc.SetTextColor(255, 255, 255)
	doc.SetFont("DejaVu", "B", 8.5)
	doc.SetXY(22, y+2.5)
	doc.CellFormat(166, 5, "ITENS DO ORÇAMENTO", "", 0, "L", false, 0, "")
	y += 10
	drawTableHeader := func() {
		doc.SetFillColor(13, 35, 83)
		doc.Rect(18, y, 176, 9, "F")
		doc.SetTextColor(255, 255, 255)
		doc.SetFont("DejaVu", "B", 7)
		for _, col := range []struct {
			x, w         float64
			label, align string
		}{{22, 13, "ITEM", "L"}, {36, 82, "DESCRIÇÃO DO SERVIÇO", "L"}, {120, 17, "QTDE", "R"}, {138, 28, "VALOR UNITÁRIO", "R"}, {168, 24, "SUBTOTAL", "R"}} {
			doc.SetXY(col.x, y+2)
			doc.CellFormat(col.w, 5, col.label, "", 0, col.align, false, 0, "")
		}
		y += 9
	}
	drawTableHeader()
	doc.SetDrawColor(130, 135, 145)
	doc.SetFont("DejaVu", "", 7.2)
	for index, item := range budget.Items {
		description := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(item.Description, "\r", " "), "\n", " "))
		if description == "" {
			description = "Serviço não informado"
		}
		lines := doc.SplitText(description, 78)
		height := maxFloat(14, float64(len(lines))*4+6)
		if y+height > 222 {
			doc.AddPage()
			pageHeader()
			y = 61
			drawTableHeader()
		}
		shade := 250
		if index%2 == 1 {
			shade = 242
		}
		doc.SetFillColor(shade, shade, shade)
		doc.Rect(18, y, 176, height, "FD")
		doc.SetTextColor(ink[0], ink[1], ink[2])
		doc.SetFont("DejaVu", "B", 7.2)
		doc.SetXY(22, y+4)
		doc.CellFormat(12, 5, fmt.Sprintf("%02d", index+1), "", 0, "L", false, 0, "")
		doc.SetFont("DejaVu", "", 7.2)
		doc.SetXY(36, y+3)
		doc.MultiCell(80, 4, strings.Join(lines, "\n"), "", "L", false)
		valueY := y + 4
		for _, col := range []struct {
			x, w  float64
			value string
		}{{119, 18, fmt.Sprintf("%.2f", item.Quantity)}, {137, 29, formatMoney(item.UnitPrice)}, {167, 25, formatMoney(item.TotalPrice)}} {
			doc.SetXY(col.x, valueY)
			doc.CellFormat(col.w, 5, col.value, "", 0, "R", false, 0, "")
		}
		y += height
	}
	if len(budget.Items) == 0 {
		doc.SetTextColor(ink[0], ink[1], ink[2])
		doc.SetFont("DejaVu", "", 8)
		doc.SetXY(22, y+4)
		doc.CellFormat(166, 6, "Nenhum item informado", "", 0, "L", false, 0, "")
		y += 14
	}
	if y+18 > 222 {
		doc.AddPage()
		pageHeader()
		y = 61
	}
	doc.SetFillColor(239, 243, 249)
	doc.RoundedRect(128, y, 66, 13, 1.5, "1234", "FD")
	doc.SetTextColor(navy[0], navy[1], navy[2])
	doc.SetFont("DejaVu", "B", 8)
	doc.SetXY(132, y+4)
	doc.CellFormat(29, 5, "VALOR TOTAL:", "", 0, "L", false, 0, "")
	doc.SetFont("DejaVu", "B", 11)
	doc.SetXY(160, y+3)
	doc.CellFormat(31, 6, formatMoney(budget.FinalValue), "", 0, "R", false, 0, "")
	y += 18
	if y+58 > 250 {
		doc.AddPage()
		pageHeader()
		y = 61
	}

	boxTop := y
	drawConditionsBox(doc, budget, boxTop)
	drawBudgetTechnicianSignature(doc, "budget-signature", boxTop)
	for page := 1; page <= doc.PageCount(); page++ {
		doc.PageNo()
		// gofpdf selects a page via SetPage; add the shared branded footer to all pages.
		doc.SetPage(page)
		doc.ImageOptions("budget-footer", 8, 254, 194, 35, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	}
	if doc.Error() != nil {
		return nil, doc.Error()
	}
	var output bytes.Buffer
	if err := doc.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func drawConditionsBox(doc *gofpdf.Fpdf, budget domain.BudgetEstimate, y float64) {
	navy, ink := []int{9, 30, 78}, []int{35, 39, 48}
	doc.SetFillColor(255, 255, 255)
	doc.SetDrawColor(130, 135, 145)
	doc.RoundedRect(16, y, 86, 52, 1.5, "1234", "FD")
	doc.SetFillColor(navy[0], navy[1], navy[2])
	doc.RoundedRect(16, y, 86, 9, 1.5, "1234", "F")
	doc.SetTextColor(255, 255, 255)
	doc.SetFont("DejaVu", "B", 7.8)
	doc.SetXY(22, y+2)
	doc.CellFormat(76, 5, "CONDIÇÕES", "", 0, "L", false, 0, "")
	validity := budgetValidityDays(budget.Date, budget.ValidUntil)
	conditions := []string{
		fmt.Sprintf("• Este orçamento tem validade de %d dias, até %s.", validity, formatDate(budget.ValidUntil)),
		"• O valor refere-se apenas aos serviços descritos.",
		"• Materiais, peças e acessórios serão cobrados à parte.",
		"• Garantia parametrizada: " + safe(budget.WarrantyTerms) + ".",
	}
	doc.SetTextColor(ink[0], ink[1], ink[2])
	doc.SetFont("DejaVu", "", 6.6)
	for index, condition := range conditions {
		doc.SetXY(20, y+12+float64(index)*8)
		doc.MultiCell(78, 4, condition, "", "L", false)
	}
}

func drawBudgetTechnicianSignature(doc *gofpdf.Fpdf, imageName string, y float64) {
	navy, ink := []int{9, 30, 78}, []int{35, 39, 48}
	doc.SetFillColor(255, 255, 255)
	doc.SetDrawColor(130, 135, 145)
	doc.RoundedRect(108, y, 86, 52, 1.5, "1234", "FD")
	doc.SetFillColor(navy[0], navy[1], navy[2])
	doc.RoundedRect(108, y, 86, 9, 1.5, "1234", "F")
	doc.SetTextColor(255, 255, 255)
	doc.SetFont("DejaVu", "B", 7.8)
	doc.SetXY(114, y+2)
	doc.CellFormat(76, 5, "RESPONSÁVEL TÉCNICO", "", 0, "L", false, 0, "")
	info := doc.GetImageInfo(imageName)
	if info != nil && info.Width() > 0 && info.Height() > 0 {
		width, height := 58.0, 16.0
		ratio := info.Width() / info.Height()
		if heightFromWidth := width / ratio; heightFromWidth < height {
			height = heightFromWidth
		} else {
			width = height * ratio
		}
		doc.ImageOptions(imageName, 151-width/2, y+11+(16-height)/2, width, height, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	}
	doc.SetDrawColor(130, 135, 145)
	doc.Line(120, y+37, 182, y+37)
	doc.SetTextColor(ink[0], ink[1], ink[2])
	doc.SetFont("DejaVu", "B", 7)
	doc.SetXY(114, y+39)
	doc.CellFormat(74, 4, "Gabriel Nascimento", "", 0, "C", false, 0, "")
	doc.SetFont("DejaVu", "", 6.5)
	doc.SetXY(114, y+44)
	doc.CellFormat(74, 4, "Técnico Responsável", "", 0, "C", false, 0, "")
}

func budgetValidityDays(issued, expiry string) int {
	start, startErr := time.Parse("2006-01-02", strings.TrimSpace(issued))
	end, endErr := time.Parse("2006-01-02", strings.TrimSpace(expiry))
	if startErr != nil || endErr != nil || end.Before(start) {
		return 0
	}
	return int(end.Sub(start).Hours() / 24)
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
