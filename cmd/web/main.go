//go:build !js || !wasm

package main

import (
	"log"
	"os"

	"inovarapp/core/ui/webapp"
)

func main() {
	if err := webapp.RunWeb(webapp.ConfiguredServerAddress(os.Getenv("PORT"))); err != nil {
		log.Fatal(err)
	}
}
