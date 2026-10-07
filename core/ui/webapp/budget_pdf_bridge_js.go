//go:build js

package webapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"syscall/js"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func requestBudgetPDF(ctx app.Context, endpoint, token, budgetID string) error {
	body, _ := json.Marshal(map[string]string{"budget_id": budgetID})
	options := js.Global().Get("Object").New()
	options.Set("method", http.MethodPost)
	headers := js.Global().Get("Headers").New()
	headers.Call("set", "Authorization", "Bearer "+token)
	headers.Call("set", "Content-Type", "application/json")
	options.Set("headers", headers)
	options.Set("body", string(body))
	resultChannel := make(chan error, 1)
	var onResponse, onError js.Func
	onResponse = js.FuncOf(func(this js.Value, args []js.Value) any {
		response := args[0]
		if !response.Get("ok").Bool() {
			var onText js.Func
			onText = js.FuncOf(func(this js.Value, textArgs []js.Value) any {
				message := "falha ao gerar PDF do orçamento"
				if len(textArgs) > 0 && textArgs[0].String() != "" {
					message = textArgs[0].String()
				}
				resultChannel <- fmt.Errorf("%s", message)
				onText.Release()
				onResponse.Release()
				onError.Release()
				return nil
			})
			response.Call("text").Call("then", onText)
			return nil
		}
		var onBlob js.Func
		onBlob = js.FuncOf(func(this js.Value, blobArgs []js.Value) any {
			blob := blobArgs[0]
			fallbackName := budgetID + "-orcamento.pdf"
			disposition := response.Get("headers").Call("get", "Content-Disposition").String()
			fileName := budgetPDFDownloadFilename(disposition, fallbackName)
			if handled, saveErr := saveDesktopFileFromBlob(fileName, blob); handled {
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
			link.Set("download", fileName)
			document.Get("body").Call("appendChild", link)
			link.Call("click")
			link.Call("remove")
			var revoke js.Func
			revoke = js.FuncOf(func(this js.Value, args []js.Value) any {
				js.Global().Get("URL").Call("revokeObjectURL", objectURL)
				revoke.Release()
				return nil
			})
			js.Global().Call("setTimeout", revoke, 1000)
			resultChannel <- nil
			onBlob.Release()
			return nil
		})
		response.Call("blob").Call("then", onBlob)
		return nil
	})
	onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		resultChannel <- errors.New("falha de rede ao gerar PDF")
		onResponse.Release()
		onError.Release()
		return nil
	})
	js.Global().Call("fetch", endpoint, options).Call("then", onResponse).Call("catch", onError)
	return <-resultChannel
}
