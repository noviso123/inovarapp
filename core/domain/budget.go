package domain

import "math"

type BudgetTotals struct {
	Subtotal       float64
	Discount       float64
	FinalValue     float64
	LaborValue     float64
	MaterialsValue float64
}

// BudgetLineTotal mirrors the quantity × unit-price calculation in the current form.
func BudgetLineTotal(quantity, unitPrice float64) float64 {
	return numberOrZero(quantity) * numberOrZero(unitPrice)
}

// CalculateBudgetTotals mirrors the current UI and Supabase column split:
// service lines count as labor; parts and materials count as materials.
func CalculateBudgetTotals(items []BudgetItem, discount float64) BudgetTotals {
	var totals BudgetTotals
	for _, item := range items {
		line := numberOrZero(item.TotalPrice)
		totals.Subtotal += line
		if item.Category == BudgetItemService {
			totals.LaborValue += line
		} else {
			totals.MaterialsValue += line
		}
	}
	totals.Discount = numberOrZero(discount)
	totals.FinalValue = math.Max(0, totals.Subtotal-totals.Discount)
	return totals
}

func numberOrZero(value float64) float64 {
	if math.IsNaN(value) || value == 0 {
		return 0
	}
	return value
}
