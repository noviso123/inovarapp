package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidServiceCompletion = errors.New("invalid service completion")

type ServiceCompletion struct {
	Date          string
	ReturnDate    string
	WarrantyDays  int
	Total         float64
	ChecklistJSON string
	Observations  string
}

// BuildServiceCompletion centralizes the legacy OS completion markers for all
// platform clients. Keep the marker names and order stable for old readers.
func BuildServiceCompletion(serviceType, payment string, checklist any, laborPrice, partsPrice float64, warrantyDays, returnMonths int, now time.Time) (ServiceCompletion, error) {
	serviceType = strings.TrimSpace(serviceType)
	payment = strings.TrimSpace(payment)
	if serviceType == "" || payment == "" || now.IsZero() {
		return ServiceCompletion{}, ErrInvalidServiceCompletion
	}
	checklistBytes, err := json.Marshal(checklist)
	if err != nil {
		return ServiceCompletion{}, fmt.Errorf("encode service checklist: %w", err)
	}
	if warrantyDays < 0 {
		warrantyDays = 0
	}
	date := now.Format("2006-01-02")
	returnDate := AddMonthsClamped(now, returnMonths).Format("2006-01-02")
	observations := fmt.Sprintf("Serviço de campo concluído (%s). [GARANTIA_DIAS:%d] Garantia de %d dias. Pagamento: %s. [PROXIMO_RETORNO:%s] [CHECKLIST:%s] [MAO_OBRA:%s] [PECAS:%s]",
		serviceType, warrantyDays, warrantyDays, payment, returnDate, string(checklistBytes),
		strconv.FormatFloat(laborPrice, 'f', -1, 64), strconv.FormatFloat(partsPrice, 'f', -1, 64))
	return ServiceCompletion{
		Date: date, ReturnDate: returnDate, WarrantyDays: warrantyDays,
		Total: laborPrice + partsPrice, ChecklistJSON: string(checklistBytes), Observations: observations,
	}, nil
}
