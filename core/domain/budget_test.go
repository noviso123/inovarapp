package domain

import (
	"math"
	"testing"
)

func TestBudgetLineTotal(t *testing.T) {
	if got := BudgetLineTotal(2, 125.5); got != 251 {
		t.Fatalf("line total = %v, want 251", got)
	}
	if got := BudgetLineTotal(math.NaN(), 50); got != 0 {
		t.Fatalf("invalid quantity should follow JavaScript fallback to zero; got %v", got)
	}
}

func TestCalculateBudgetTotalsAndSupabaseSplit(t *testing.T) {
	items := []BudgetItem{
		{Quantity: 2, UnitPrice: 100, TotalPrice: 200, Category: BudgetItemService},
		{Quantity: 3, UnitPrice: 25, TotalPrice: 75, Category: BudgetItemPart},
		{Quantity: 1, UnitPrice: 25, TotalPrice: 25, Category: BudgetItemMaterial},
	}
	totals := CalculateBudgetTotals(items, 30)
	if totals.Subtotal != 300 || totals.Discount != 30 || totals.FinalValue != 270 || totals.LaborValue != 200 || totals.MaterialsValue != 100 {
		t.Fatalf("unexpected budget totals: %#v", totals)
	}
}

func TestCalculateBudgetTotalsFloorsFinalAtZero(t *testing.T) {
	totals := CalculateBudgetTotals([]BudgetItem{{TotalPrice: 50, Category: BudgetItemService}}, 75)
	if totals.FinalValue != 0 {
		t.Fatalf("final value = %v, want 0", totals.FinalValue)
	}
	negativeDiscount := CalculateBudgetTotals([]BudgetItem{{TotalPrice: 50}}, -10)
	if negativeDiscount.FinalValue != 60 {
		t.Fatalf("negative discount behavior changed: %#v", negativeDiscount)
	}
}
