//go:build !js

package webapp

import (
	"errors"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func requestServiceOrderPDF(ctx app.Context, endpoint, token, serviceID string, sendWhatsApp bool) serviceOrderPDFResult {
	return serviceOrderPDFResult{err: errors.New("geração de PDF só está disponível no navegador")}
}
