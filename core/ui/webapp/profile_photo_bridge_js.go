//go:build js && wasm

package webapp

import "syscall/js"

type customerProfilePhotoInput struct {
	DataURL string
	Error   string
}

func startCustomerProfilePhotoPicker() <-chan *customerProfilePhotoInput {
	out := make(chan *customerProfilePhotoInput, 1)
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", "image/*")
	input.Get("style").Set("display", "none")
	doc.Get("body").Call("appendChild", input)
	var change, cancel js.Func
	finished := false
	finish := func(result *customerProfilePhotoInput) {
		if finished {
			return
		}
		finished = true
		out <- result
		input.Call("remove")
		if change.Value.Type() == js.TypeFunction {
			change.Release()
		}
		if cancel.Value.Type() == js.TypeFunction {
			cancel.Release()
		}
	}
	cancel = js.FuncOf(func(this js.Value, args []js.Value) any { finish(nil); return nil })
	change = js.FuncOf(func(this js.Value, args []js.Value) any {
		files := input.Get("files")
		if files.Get("length").Int() == 0 {
			finish(nil)
			return nil
		}
		file := files.Index(0)
		reader := js.Global().Get("FileReader").New()
		var loaded, readError js.Func
		readError = js.FuncOf(func(this js.Value, args []js.Value) any {
			loaded.Release()
			readError.Release()
			finish(&customerProfilePhotoInput{Error: "Falha ao ler a imagem"})
			return nil
		})
		loaded = js.FuncOf(func(this js.Value, args []js.Value) any {
			raw := args[0].Get("target").Get("result").String()
			image := js.Global().Get("Image").New()
			var onload, onerror js.Func
			onerror = js.FuncOf(func(this js.Value, args []js.Value) any {
				onload.Release()
				onerror.Release()
				loaded.Release()
				readError.Release()
				finish(&customerProfilePhotoInput{Error: "Formato de imagem não suportado"})
				return nil
			})
			onload = js.FuncOf(func(this js.Value, args []js.Value) any {
				width, height := image.Get("naturalWidth").Int(), image.Get("naturalHeight").Int()
				scale := 1.0
				if max(width, height) > 320 {
					scale = 320.0 / float64(max(width, height))
				}
				canvas := doc.Call("createElement", "canvas")
				canvas.Set("width", max(1, int(float64(width)*scale+0.5)))
				canvas.Set("height", max(1, int(float64(height)*scale+0.5)))
				context := canvas.Call("getContext", "2d")
				dataURL := raw
				if !context.IsNull() && !context.IsUndefined() {
					context.Call("drawImage", image, 0, 0, canvas.Get("width"), canvas.Get("height"))
					dataURL = canvas.Call("toDataURL", "image/jpeg", 0.85).String()
				}
				image.Set("onload", js.Null())
				image.Set("onerror", js.Null())
				onload.Release()
				onerror.Release()
				loaded.Release()
				readError.Release()
				finish(&customerProfilePhotoInput{DataURL: dataURL})
				return nil
			})
			image.Set("onload", onload)
			image.Set("onerror", onerror)
			image.Set("src", raw)
			return nil
		})
		reader.Set("onload", loaded)
		reader.Set("onerror", readError)
		reader.Call("readAsDataURL", file)
		return nil
	})
	input.Call("addEventListener", "change", change)
	input.Call("addEventListener", "cancel", cancel)
	input.Call("click")
	return out
}
