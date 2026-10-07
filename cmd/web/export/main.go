package main

import (
	"flag"
	"log"

	"inovarapp/core/ui/webapp"
)

func main() {
	output := flag.String("out", "mobile/www", "diretório de saída para o WebView nativo")
	flag.Parse()
	if err := webapp.ExportStatic(*output); err != nil {
		log.Fatal(err)
	}
}
