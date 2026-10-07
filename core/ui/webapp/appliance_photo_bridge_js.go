//go:build js && wasm

package webapp

import "syscall/js"

func startAppliancePhotoPicker() <-chan []appliancePhotoInput {
	return startAppliancePhotoPickerLimit(0)
}

func startAppliancePhotoPickerLimit(limit int) <-chan []appliancePhotoInput {
	out := make(chan []appliancePhotoInput, 1)
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", "image/*")
	input.Set("multiple", true)
	input.Get("style").Set("display", "none")
	doc.Get("body").Call("appendChild", input)
	var change js.Func
	var cancel js.Func
	var finish func([]appliancePhotoInput)
	finished := false
	finish = func(items []appliancePhotoInput) {
		if finished {
			return
		}
		finished = true
		out <- items
		input.Call("remove")
		if change.Value.Type() == js.TypeFunction {
			change.Release()
		}
		cancel.Release()
	}
	cancel = js.FuncOf(func(this js.Value, args []js.Value) any { finish(nil); return nil })
	input.Call("addEventListener", "cancel", cancel)
	change = js.FuncOf(func(this js.Value, args []js.Value) any {
		files := input.Get("files")
		count := files.Get("length").Int()
		if limit > 0 && count > limit {
			count = limit
		}
		items := make([]appliancePhotoInput, 0, count)
		var read func(int)
		read = func(index int) {
			if index >= count {
				finish(items)
				return
			}
			file := files.Index(index)
			reader := js.Global().Get("FileReader").New()
			var loaded js.Func
			var readError js.Func
			readError = js.FuncOf(func(this js.Value, args []js.Value) any {
				name := file.Get("name").String()
				if name == "" {
					name = "foto.jpg"
				}
				items = append(items, appliancePhotoInput{Name: name, Type: file.Get("type").String(), Size: int64(file.Get("size").Int()), Error: true})
				readError.Release()
				loaded.Release()
				read(index + 1)
				return nil
			})
			loaded = js.FuncOf(func(this js.Value, args []js.Value) any {
				image := js.Global().Get("Image").New()
				var onload js.Func
				var onerror js.Func
				onerror = js.FuncOf(func(this js.Value, args []js.Value) any {
					original := reader.Get("result").String()
					name := file.Get("name").String()
					if name == "" {
						name = "foto.jpg"
					}
					items = append(items, appliancePhotoInput{Name: name, Base64: original, OriginalDataURL: original, Type: file.Get("type").String(), Size: int64(file.Get("size").Int())})
					loaded.Release()
					onload.Release()
					onerror.Release()
					read(index + 1)
					return nil
				})
				onload = js.FuncOf(func(this js.Value, args []js.Value) any {
					w, h := image.Get("naturalWidth").Int(), image.Get("naturalHeight").Int()
					original := args[0].Get("target").Get("result").String()
					if w > 1600 || h > 1600 {
						scale := 1600.0 / float64(max(w, h))
						w, h = int(float64(w)*scale), int(float64(h)*scale)
					}
					canvas := doc.Call("createElement", "canvas")
					canvas.Set("width", w)
					canvas.Set("height", h)
					canvasContext := canvas.Call("getContext", "2d")
					if canvasContext.IsNull() || canvasContext.IsUndefined() {
						name := file.Get("name").String()
						if name == "" {
							name = "foto.jpg"
						}
						items = append(items, appliancePhotoInput{Name: name, Base64: original, OriginalDataURL: original, Type: file.Get("type").String(), Size: int64(file.Get("size").Int())})
						image.Set("onload", js.Null())
						image.Set("onerror", js.Null())
						onload.Release()
						onerror.Release()
						loaded.Release()
						read(index + 1)
						return nil
					}
					canvasContext.Call("drawImage", image, 0, 0, w, h)
					name := file.Get("name").String()
					if len(name) == 0 {
						name = "foto.jpg"
					}
					compressed := js.Global().Get("Function").New("canvas", "try { return canvas.toDataURL('image/jpeg', 0.82) } catch (e) { return '' }").Invoke(canvas).String()
					if compressed == "" {
						compressed = original
					}
					items = append(items, appliancePhotoInput{
						Name: name, Base64: compressed,
						OriginalDataURL: original,
						Type:            file.Get("type").String(), Size: int64(file.Get("size").Int()),
					})
					image.Set("onload", js.Null())
					image.Set("onerror", js.Null())
					onload.Release()
					onerror.Release()
					loaded.Release()
					read(index + 1)
					return nil
				})
				image.Set("onload", onload)
				image.Set("onerror", onerror)
				image.Set("src", args[0].Get("target").Get("result"))
				return nil
			})
			reader.Set("onload", loaded)
			reader.Set("onerror", readError)
			reader.Call("readAsDataURL", file)

		}
		if count == 0 {
			finish(nil)
			return nil
		}
		read(0)
		return nil
	})
	input.Call("addEventListener", "change", change)
	input.Call("click")
	return out
}
