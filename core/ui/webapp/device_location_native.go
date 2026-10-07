//go:build !js || !wasm

package webapp

import "errors"

type deviceLocation struct {
	latitude  float64
	longitude float64
	accuracy  float64
}

type deviceLocationResult struct {
	location deviceLocation
	err      error
}

func requestDeviceLocation() <-chan deviceLocationResult {
	result := make(chan deviceLocationResult, 1)
	result <- deviceLocationResult{err: errors.New("A localização só está disponível na versão interativa do aplicativo.")}
	return result
}
