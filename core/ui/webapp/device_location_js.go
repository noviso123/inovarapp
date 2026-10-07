//go:build js && wasm

package webapp

import (
	"errors"
	"syscall/js"
)

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
	promise, err := beginDeviceLocationRequest()
	if err != nil {
		result <- deviceLocationResult{err: err}
		return result
	}
	go func() {
		location, err := awaitDeviceLocationRequest(promise)
		result <- deviceLocationResult{location: location, err: err}
	}()
	return result
}

func beginDeviceLocationRequest() (js.Value, error) {
	window := js.Global().Get("window")
	if window.Type() == js.TypeObject && window.Get("inovarGetCurrentLocation").Type() == js.TypeFunction {
		return window.Call("inovarGetCurrentLocation"), nil
	}

	navigator := js.Global().Get("navigator")
	if navigator.Type() != js.TypeObject || navigator.Get("geolocation").Type() != js.TypeObject {
		return js.Undefined(), errors.New("Este dispositivo ou navegador não oferece acesso à localização.")
	}
	var resolve, reject js.Value
	promiseExecutor := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve = args[0]
		reject = args[1]
		return nil
	})
	promise := js.Global().Get("Promise").New(promiseExecutor)
	promiseExecutor.Release()

	var success, failure js.Func
	success = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			resolve.Invoke(args[0])
		}
		success.Release()
		failure.Release()
		return nil
	})
	failure = js.FuncOf(func(_ js.Value, args []js.Value) any {
		message := "Não foi possível obter a localização."
		if len(args) > 0 && args[0].Get("code").Type() == js.TypeNumber {
			switch args[0].Get("code").Int() {
			case 1:
				message = "A permissão de localização foi negada. Ative-a nas configurações do navegador ou do sistema."
			case 2:
				message = "A localização está indisponível. Verifique se os serviços de localização estão ativos neste dispositivo."
			case 3:
				message = "A localização demorou para responder. Tente novamente em um local com melhor sinal."
			}
		} else if len(args) > 0 && args[0].Get("message").Type() == js.TypeString {
			message = args[0].Get("message").String()
		}
		reject.Invoke(js.Global().Get("Error").New(message))
		success.Release()
		failure.Release()
		return nil
	})
	options := js.Global().Get("Object").New()
	options.Set("enableHighAccuracy", false)
	options.Set("timeout", 20000)
	options.Set("maximumAge", 300000)
	navigator.Get("geolocation").Call("getCurrentPosition", success, failure, options)
	return promise, nil
}

func awaitDeviceLocationRequest(promise js.Value) (deviceLocation, error) {
	position, err := awaitJSPromise(promise)
	if err != nil {
		return deviceLocation{}, friendlyLocationError(err)
	}
	coords := position.Get("coords")
	if coords.Type() != js.TypeObject {
		return deviceLocation{}, errors.New("O dispositivo não retornou coordenadas válidas.")
	}
	return deviceLocation{latitude: coords.Get("latitude").Float(), longitude: coords.Get("longitude").Float(), accuracy: coords.Get("accuracy").Float()}, nil
}

func deviceLocationFromJS(value js.Value) (deviceLocation, error) {
	if value.Type() != js.TypeObject || value.Get("latitude").Type() != js.TypeNumber || value.Get("longitude").Type() != js.TypeNumber {
		return deviceLocation{}, errors.New("O dispositivo não retornou coordenadas válidas.")
	}
	accuracy := 0.0
	if value.Get("accuracy").Type() == js.TypeNumber {
		accuracy = value.Get("accuracy").Float()
	}
	return deviceLocation{latitude: value.Get("latitude").Float(), longitude: value.Get("longitude").Float(), accuracy: accuracy}, nil
}

func friendlyLocationError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if message == "Permission denied" || message == "User denied Geolocation" {
		return errors.New("A permissão de localização foi negada. Ative-a nas configurações do navegador ou do sistema.")
	}
	return err
}
