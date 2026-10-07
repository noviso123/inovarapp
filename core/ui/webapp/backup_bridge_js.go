//go:build js

package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"syscall/js"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func selectAndRestoreTeamBackup(ctx app.Context, endpoint, token string) (string, error) {
	type result struct {
		message string
		err     error
	}
	resultChannel := make(chan result, 1)
	document := js.Global().Get("document")
	input := document.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".json,application/json")
	input.Get("style").Set("display", "none")
	document.Get("body").Call("appendChild", input)
	var change, cancel js.Func
	finished := false
	finish := func(message string, err error) {
		if finished {
			return
		}
		finished = true
		resultChannel <- result{message, err}
		input.Call("remove")
		change.Release()
		cancel.Release()
	}
	cancel = js.FuncOf(func(this js.Value, args []js.Value) any {
		finish("", errors.New("Seleção do backup cancelada."))
		return nil
	})
	input.Call("addEventListener", "cancel", cancel)
	change = js.FuncOf(func(this js.Value, args []js.Value) any {
		files := input.Get("files")
		if files.Get("length").Int() == 0 {
			finish("", errors.New("Nenhum arquivo foi selecionado."))
			return nil
		}
		file := files.Index(0)
		if file.Get("size").Int() > 32<<20 {
			finish("", errors.New("O arquivo excede o limite de 32 MB."))
			return nil
		}
		if !js.Global().Call("confirm", "Restaurar este backup? Os registros com o mesmo ID serão atualizados; os demais serão adicionados. Nenhum registro será excluído.").Bool() {
			finish("", errors.New("Restauração cancelada."))
			return nil
		}
		reader := js.Global().Get("FileReader").New()
		var onLoad, onError js.Func
		onError = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish("", errors.New("Não foi possível ler o arquivo selecionado."))
			onError.Release()
			onLoad.Release()
			return nil
		})
		onLoad = js.FuncOf(func(this js.Value, args []js.Value) any {
			options := js.Global().Get("Object").New()
			options.Set("method", http.MethodPost)
			headers := js.Global().Get("Headers").New()
			headers.Call("set", "Authorization", "Bearer "+token)
			headers.Call("set", "Content-Type", "application/json")
			options.Set("headers", headers)
			options.Set("body", reader.Get("result").String())
			var onResponse, onFetchError js.Func
			onResponse = js.FuncOf(func(this js.Value, args []js.Value) any {
				response := args[0]
				var onText js.Func
				onText = js.FuncOf(func(this js.Value, textArgs []js.Value) any {
					text := textArgs[0].String()
					if !response.Get("ok").Bool() {
						finish("", fmt.Errorf("Falha na restauração: %s", text))
					} else {
						finish("Restauração concluída. Atualize a tela para conferir os dados.", nil)
					}
					onText.Release()
					onResponse.Release()
					onFetchError.Release()
					return nil
				})
				response.Call("text").Call("then", onText)
				return nil
			})
			onFetchError = js.FuncOf(func(this js.Value, args []js.Value) any {
				finish("", errors.New("Falha de rede durante a restauração. Você pode reenviar o mesmo backup."))
				onResponse.Release()
				onFetchError.Release()
				return nil
			})
			js.Global().Call("fetch", endpoint, options).Call("then", onResponse).Call("catch", onFetchError)
			onLoad.Release()
			onError.Release()
			return nil
		})
		reader.Set("onload", onLoad)
		reader.Set("onerror", onError)
		reader.Call("readAsText", file, "UTF-8")
		return nil
	})
	input.Call("addEventListener", "change", change)
	input.Call("click")
	resultValue := <-resultChannel
	return strings.TrimSpace(resultValue.message), resultValue.err
}

func downloadTeamBackup(ctx app.Context, endpoint, token string) error {
	options := js.Global().Get("Object").New()
	options.Set("method", http.MethodGet)
	headers := js.Global().Get("Headers").New()
	headers.Call("set", "Authorization", "Bearer "+token)
	options.Set("headers", headers)
	resultChannel := make(chan error, 1)
	var onResponse, onError js.Func
	onResponse = js.FuncOf(func(this js.Value, args []js.Value) any {
		response := args[0]
		if !response.Get("ok").Bool() {
			resultChannel <- errors.New("backup request failed")
			onResponse.Release()
			onError.Release()
			return nil
		}
		var onBlob js.Func
		onBlob = js.FuncOf(func(this js.Value, blobArgs []js.Value) any {
			blob := blobArgs[0]
			contentDisposition := response.Get("headers").Call("get", "Content-Disposition")
			filename := "inovarapp-backup.json"
			if !contentDisposition.IsNull() && contentDisposition.String() != "" {
				for _, part := range strings.Split(contentDisposition.String(), ";") {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "filename=") {
						filename = strings.Trim(strings.TrimPrefix(part, "filename="), "\"")
					}
				}
			}
			if handled, saveErr := saveDesktopFileFromBlob(filename, blob); handled {
				resultChannel <- saveErr
				onBlob.Release()
				onResponse.Release()
				onError.Release()
				return nil
			}
			objectURL := js.Global().Get("URL").Call("createObjectURL", blob).String()
			document := js.Global().Get("document")
			link := document.Call("createElement", "a")
			link.Set("href", objectURL)
			link.Set("download", filename)
			document.Get("body").Call("appendChild", link)
			link.Call("click")
			link.Call("remove")
			js.Global().Get("URL").Call("revokeObjectURL", objectURL)
			resultChannel <- nil
			onBlob.Release()
			onResponse.Release()
			return nil
		})
		response.Call("blob").Call("then", onBlob)
		return nil
	})
	onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		resultChannel <- errors.New("backup network request failed")
		onResponse.Release()
		onError.Release()
		return nil
	})
	js.Global().Call("fetch", endpoint, options).Call("then", onResponse).Call("catch", onError)
	return <-resultChannel
}
