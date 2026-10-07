//go:build !js

package webapp

import (
	"errors"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func requestBudgetPDF(ctx app.Context, endpoint, token, budgetID string) error {
	return errors.New("download do PDF disponível somente na interface web")
}
