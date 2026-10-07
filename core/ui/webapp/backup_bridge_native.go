//go:build !js

package webapp

import (
	"errors"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func downloadTeamBackup(ctx app.Context, endpoint, token string) error {
	return errors.New("download do backup disponível somente na interface web")
}

func selectAndRestoreTeamBackup(ctx app.Context, endpoint, token string) (string, error) {
	return "", errors.New("restauração disponível somente na interface web")
}
