//go:build !js || !wasm

package webapp

import (
	"embed"
	"io/fs"
)

//go:embed static/web/*
var packagedWebAssets embed.FS

func packagedWebFS() (fs.FS, error) { return fs.Sub(packagedWebAssets, "static/web") }
