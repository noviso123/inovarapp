//go:build js && wasm

package webapp

import (
	"errors"
	"io/fs"
)

func packagedWebFS() (fs.FS, error) {
	return nil, errors.New("arquivos PWA são servidos pelo host WebAssembly")
}
