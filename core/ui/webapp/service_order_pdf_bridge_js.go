//go:build js

package webapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"syscall/js"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

var serviceOrderPDFTimerCallbacks []js.Func

func requestServiceOrderPDF(ctx app.Context, endpoint, token, serviceID string, sendWhatsApp bool) serviceOrderPDFResult {
	request, _ := json.Marshal(map[string]any{"service_id": serviceID, "send_whatsapp": sendWhatsApp})
	options := js.Global().Get("Object").New()
	options.Set("method", http.MethodPost)
	headers := js.Global().Get("Headers").New()
	headers.Call("set", "Authorization", "Bearer "+token)
	headers.Call("set", "Content-Type", "application/json")
	options.Set("headers", headers)
	options.Set("body", string(request))
	pdfBridgeResultChannel := make(chan serviceOrderPDFResult, 1)
	var onResponse, onError js.Func
	onResponse = js.FuncOf(func(this js.Value, args []js.Value) any {
		response := args[0]
		if !response.Get("ok").Bool() {
			var onText js.Func
			onText = js.FuncOf(func(this js.Value, textArgs []js.Value) any {
				message := "falha ao gerar PDF"
				if len(textArgs) > 0 && textArgs[0].String() != "" {
					message = textArgs[0].String()
				}
				pdfBridgeResultChannel <- serviceOrderPDFResult{status: response.Get("status").Int(), err: fmt.Errorf("%s", message)}
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
			fileNameValue := response.Get("headers").Call("get", "X-Service-Order-File")
			fileName := ""
			if fileNameValue.Type() == js.TypeString {
				fileName = fileNameValue.String()
			}
			if fileName == "" {
				fileName = "ordem-de-servico.pdf"
			}
			if handled, saveErr := saveDesktopFileFromBlob(fileName, blob); handled {
				if saveErr != nil {
					pdfBridgeResultChannel <- serviceOrderPDFResult{status: response.Get("status").Int(), err: saveErr}
				} else {
					pdfBridgeResultChannel <- serviceOrderPDFResult{status: response.Get("status").Int(), whatsapp: response.Get("headers").Call("get", "X-Service-Order-WhatsApp").String(), url: response.Get("headers").Call("get", "X-Service-Order-URL").String()}
				}
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
			timer := js.FuncOf(func(this js.Value, args []js.Value) any {
				js.Global().Get("URL").Call("revokeObjectURL", objectURL)
				serviceOrderPDFTimerCallbacks[len(serviceOrderPDFTimerCallbacks)-1].Release()
				serviceOrderPDFTimerCallbacks = serviceOrderPDFTimerCallbacks[:len(serviceOrderPDFTimerCallbacks)-1]
				return nil
			})
			serviceOrderPDFTimerCallbacks = append(serviceOrderPDFTimerCallbacks, timer)
			js.Global().Call("setTimeout", timer, 1000)
			pdfBridgeResultChannel <- serviceOrderPDFResult{status: response.Get("status").Int(), whatsapp: response.Get("headers").Call("get", "X-Service-Order-WhatsApp").String(), url: response.Get("headers").Call("get", "X-Service-Order-URL").String()}
			onBlob.Release()
			return nil
		})
		response.Call("blob").Call("then", onBlob)
		return nil
	})
	onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		pdfBridgeResultChannel <- serviceOrderPDFResult{err: fmt.Errorf("falha de rede ao gerar PDF")}
		return nil
	})
	js.Global().Call("fetch", endpoint, options).Call("then", onResponse).Call("catch", onError)
	result := <-pdfBridgeResultChannel
	onResponse.Release()
	onError.Release()
	return result
}
