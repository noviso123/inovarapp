package domain

import (
	"errors"
	"strings"
)

type BudgetResponseAction string

const (
	BudgetResponseApprove BudgetResponseAction = "APROVAR"
	BudgetResponseDecline BudgetResponseAction = "RECUSAR"
)

var (
	ErrInvalidBudgetResponseAction = errors.New("invalid budget response action")
	ErrBudgetNotOwned              = errors.New("budget does not belong to caller")
	ErrBudgetSignatureRequired     = errors.New("customer approval requires a signature")
	ErrBudgetAlreadyAnswered       = errors.New("budget already answered")
	ErrInvalidBudgetResponder      = errors.New("invalid budget responder")
)

// ParseBudgetResponseAction applies the existing default action and casing.
func ParseBudgetResponseAction(value string) (BudgetResponseAction, error) {
	action := BudgetResponseAction(strings.ToUpper(value))
	if action == "" {
		action = BudgetResponseApprove
	}
	if action != BudgetResponseApprove && action != BudgetResponseDecline {
		return "", ErrInvalidBudgetResponseAction
	}
	return action, nil
}

// PlanBudgetResponse applies the shared owner, signature, and idempotency rules.
// Persisting signatures and status remains an adapter concern because the legacy
// database stores the signature inside the JSON description column.
func PlanBudgetResponse(role Role, action BudgetResponseAction, currentStatus string, ownsBudget, hasSignature bool) (string, error) {
	if action != BudgetResponseApprove && action != BudgetResponseDecline {
		return "", ErrInvalidBudgetResponseAction
	}
	if role != RoleCustomer && role != RoleTechnician && role != RoleAdmin {
		return "", ErrInvalidBudgetResponder
	}
	if role == RoleCustomer && !ownsBudget {
		return "", ErrBudgetNotOwned
	}
	if role == RoleCustomer && action == BudgetResponseApprove && !hasSignature {
		return "", ErrBudgetSignatureRequired
	}
	if currentStatus == "APROVADO" || currentStatus == "RECUSADO" || currentStatus == "CANCELADO" {
		return "", ErrBudgetAlreadyAnswered
	}
	if action == BudgetResponseApprove {
		return "APROVADO", nil
	}
	return "RECUSADO", nil
}
