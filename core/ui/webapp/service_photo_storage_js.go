//go:build js && wasm

package webapp

import (
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"github.com/google/uuid"
)

const localPhotoDatabase = "inovarapp_attachments"
const localPhotoStore = "files"

func saveLocalServicePhoto(serviceID string, input appliancePhotoInput) error {
	dataURL := input.OriginalDataURL
	if dataURL == "" {
		dataURL = input.Base64
	}
	if serviceID == "" || dataURL == "" {
		return fmt.Errorf("service id and photo data are required")
	}
	name := input.Name
	if name == "" {
		name = "foto.jpg"
	}
	typeName := input.Type
	if typeName == "" {
		typeName = "image/jpeg"
	}
	item := localServicePhoto{
		ID:        fmt.Sprintf("%s_%d_%s", serviceID, time.Now().UnixMilli(), uuid.NewString()),
		ServiceID: serviceID, Name: name, Type: typeName, Size: input.Size,
		DataURL: dataURL, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	_, err := attachmentDBRequest("put", item, "")
	return err
}

func listLocalServicePhotos(serviceID string) ([]localServicePhoto, error) {
	if serviceID == "" {
		return nil, fmt.Errorf("service id is required")
	}
	data, err := attachmentDBRequest("getAll", nil, "")
	if err != nil {
		return nil, err
	}
	var all []localServicePhoto
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	photos := make([]localServicePhoto, 0)
	for _, photo := range all {
		if photo.ServiceID == serviceID {
			photos = append(photos, photo)
		}
	}
	return photos, nil
}

func deleteLocalServicePhoto(id string) error {
	if id == "" {
		return fmt.Errorf("photo id is required")
	}
	_, err := attachmentDBRequest("delete", nil, id)
	return err
}

func attachmentDBRequest(operation string, item any, key string) ([]byte, error) {
	idb := js.Global().Get("indexedDB")
	if idb.IsUndefined() || idb.IsNull() {
		return nil, errLocalPhotoStorageUnavailable
	}
	request := idb.Call("open", localPhotoDatabase, 1)
	result := make(chan struct {
		data []byte
		err  error
	}, 1)
	var onUpgrade, onSuccess, onError js.Func
	cleanup := func() {
		onUpgrade.Release()
		onSuccess.Release()
		onError.Release()
	}
	onUpgrade = js.FuncOf(func(this js.Value, args []js.Value) any {
		db := request.Get("result")
		if !db.Get("objectStoreNames").Call("contains", localPhotoStore).Bool() {
			db.Call("createObjectStore", localPhotoStore, map[string]any{"keyPath": "id"})
		}
		return nil
	})
	onSuccess = js.FuncOf(func(this js.Value, args []js.Value) any {
		db := request.Get("result")
		mode := "readonly"
		if operation == "put" || operation == "delete" {
			mode = "readwrite"
		}
		tx := db.Call("transaction", localPhotoStore, mode)
		store := tx.Call("objectStore", localPhotoStore)
		var op js.Value
		switch operation {
		case "put":
			encoded, err := json.Marshal(item)
			if err != nil {
				db.Call("close")
				result <- struct {
					data []byte
					err  error
				}{err: err}
				return nil
			}
			value := js.Global().Get("JSON").Call("parse", string(encoded))
			op = store.Call("put", value)
		case "getAll":
			op = store.Call("getAll")
		case "delete":
			op = store.Call("delete", key)
		default:
			db.Call("close")
			result <- struct {
				data []byte
				err  error
			}{err: fmt.Errorf("unsupported IndexedDB operation %q", operation)}
			return nil
		}
		var requestSuccess, requestError js.Func
		var transactionComplete, transactionFailure js.Func
		completed := false
		var responseData []byte
		finish := func(operationErr error) {
			if completed {
				return
			}
			completed = true
			result <- struct {
				data []byte
				err  error
			}{data: responseData, err: operationErr}
			requestSuccess.Release()
			requestError.Release()
			transactionComplete.Release()
			transactionFailure.Release()
			db.Call("close")
		}
		requestSuccess = js.FuncOf(func(this js.Value, args []js.Value) any {
			if operation == "getAll" {
				encoded := js.Global().Get("JSON").Call("stringify", op.Get("result")).String()
				responseData = []byte(encoded)
			}
			return nil
		})
		requestError = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish(fmt.Errorf("IndexedDB %s failed", operation))
			return nil
		})
		transactionComplete = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish(nil)
			return nil
		})
		transactionFailure = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish(fmt.Errorf("IndexedDB %s transaction failed", operation))
			return nil
		})
		op.Set("onsuccess", requestSuccess)
		op.Set("onerror", requestError)
		tx.Set("oncomplete", transactionComplete)
		tx.Set("onabort", transactionFailure)
		tx.Set("onerror", transactionFailure)
		return nil
	})
	onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		result <- struct {
			data []byte
			err  error
		}{err: fmt.Errorf("could not open local photo database")}
		return nil
	})
	request.Set("onupgradeneeded", onUpgrade)
	request.Set("onsuccess", onSuccess)
	request.Set("onerror", onError)
	response := <-result
	cleanup()
	if response.err != nil {
		return nil, response.err
	}
	return response.data, nil
}
