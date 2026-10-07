package domain

import (
	"errors"
	"testing"
)

func TestParseBudgetResponseAction(t *testing.T) {
	cases := []struct {
		input string
		want  BudgetResponseAction
		bad   bool
	}{
		{"", BudgetResponseApprove, false},
		{"APROVAR", BudgetResponseApprove, false},
		{" aprovar ", "", true},
		{"recusar", BudgetResponseDecline, false},
		{"cancelar", "", true},
	}
	for _, test := range cases {
		got, err := ParseBudgetResponseAction(test.input)
		if test.bad && !errors.Is(err, ErrInvalidBudgetResponseAction) {
			t.Errorf("ParseBudgetResponseAction(%q) error = %v", test.input, err)
		}
		if !test.bad && (err != nil || got != test.want) {
			t.Errorf("ParseBudgetResponseAction(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
}

func TestPlanBudgetResponsePreservesOwnerSignatureAndIdempotencyRules(t *testing.T) {
	cases := []struct {
		name      string
		role      Role
		action    BudgetResponseAction
		status    string
		owner     bool
		signature bool
		want      string
		wantErr   error
	}{
		{"customer approves signed", RoleCustomer, BudgetResponseApprove, "ENVIADO", true, true, "APROVADO", nil},
		{"customer cannot approve unsigned", RoleCustomer, BudgetResponseApprove, "ENVIADO", true, false, "", ErrBudgetSignatureRequired},
		{"customer can decline unsigned", RoleCustomer, BudgetResponseDecline, "ENVIADO", true, false, "RECUSADO", nil},
		{"customer cannot respond for another owner", RoleCustomer, BudgetResponseDecline, "ENVIADO", false, false, "", ErrBudgetNotOwned},
		{"team can approve without customer signature", RoleTechnician, BudgetResponseApprove, "ENVIADO", false, false, "APROVADO", nil},
		{"admin can decline", RoleAdmin, BudgetResponseDecline, "RASCUNHO", false, false, "RECUSADO", nil},
		{"answered budget cannot be changed", RoleCustomer, BudgetResponseDecline, "APROVADO", true, false, "", ErrBudgetAlreadyAnswered},
		{"budget status stays case sensitive", RoleTechnician, BudgetResponseDecline, "aprovado", false, false, "RECUSADO", nil},
		{"unknown role rejected", Role("UNKNOWN"), BudgetResponseDecline, "ENVIADO", true, true, "", ErrInvalidBudgetResponder},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := PlanBudgetResponse(test.role, test.action, test.status, test.owner, test.signature)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v; want %v", err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("PlanBudgetResponse = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
