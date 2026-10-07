//go:build js && wasm

package webapp

import (
	"encoding/json"
	"errors"
	"syscall/js"
)

const offlineCacheDatabase = "inovarapp_offline_cache"
const offlineCacheStore = "accounts"

func saveOfflineSnapshot(snapshot offlineAccountSnapshot) error {
	return offlineSnapshotDBRequest("put", snapshot.UserID, snapshot)
}

func loadOfflineSnapshot(userID string) (offlineAccountSnapshot, error) {
	data, err := offlineSnapshotDBRead(userID)
	if err != nil {
		return offlineAccountSnapshot{}, err
	}
	snapshot, ok := decodeOfflineAccountSnapshot(data, userID)
	if !ok {
		return offlineAccountSnapshot{}, errors.New("offline snapshot is invalid or belongs to another account")
	}
	return snapshot, nil
}

func deleteOfflineSnapshot(userID string) error {
	return offlineSnapshotDBRequest("delete", userID, nil)
}

func offlineSnapshotDBRead(key string) ([]byte, error) {
	return offlineSnapshotDBOperation("get", key, nil)
}

func offlineSnapshotDBRequest(operation, key string, snapshot any) error {
	_, err := offlineSnapshotDBOperation(operation, key, snapshot)
	return err
}

func offlineSnapshotDBOperation(operation, key string, snapshot any) ([]byte, error) {
	idb := js.Global().Get("indexedDB")
	if idb.IsUndefined() || idb.IsNull() {
		return nil, errors.New("offline storage is unavailable")
	}
	open := idb.Call("open", offlineCacheDatabase, 1)
	result := make(chan struct {
		data []byte
		err  error
	}, 1)
	var onUpgrade, onOpen, onError js.Func
	cleanup := func() { onUpgrade.Release(); onOpen.Release(); onError.Release() }
	onUpgrade = js.FuncOf(func(this js.Value, args []js.Value) any {
		db := open.Get("result")
		if !db.Get("objectStoreNames").Call("contains", offlineCacheStore).Bool() {
			db.Call("createObjectStore", offlineCacheStore, map[string]any{"keyPath": "id"})
		}
		return nil
	})
	onOpen = js.FuncOf(func(this js.Value, args []js.Value) any {
		db := open.Get("result")
		mode := "readonly"
		if operation == "put" || operation == "delete" {
			mode = "readwrite"
		}
		tx := db.Call("transaction", offlineCacheStore, mode)
		store := tx.Call("objectStore", offlineCacheStore)
		var request js.Value
		switch operation {
		case "put":
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				result <- struct {
					data []byte
					err  error
				}{err: err}
				return nil
			}
			request = store.Call("put", js.Global().Get("JSON").Call("parse", string(encoded)))
		case "get":
			request = store.Call("get", key)
		case "delete":
			request = store.Call("delete", key)
		default:
			result <- struct {
				data []byte
				err  error
			}{err: errors.New("unsupported offline cache operation")}
			return nil
		}
		var requestSuccess, requestFailure, transactionComplete, transactionFailure js.Func
		completed := false
		var response []byte
		finish := func(err error) {
			if completed {
				return
			}
			completed = true
			result <- struct {
				data []byte
				err  error
			}{data: response, err: err}
			requestSuccess.Release()
			requestFailure.Release()
			transactionComplete.Release()
			transactionFailure.Release()
			db.Call("close")
		}
		requestSuccess = js.FuncOf(func(this js.Value, args []js.Value) any {
			var data []byte
			if operation == "get" {
				value := request.Get("result")
				if !value.IsUndefined() {
					data = []byte(js.Global().Get("JSON").Call("stringify", value).String())
				}
			}
			response = data
			return nil
		})
		requestFailure = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish(errors.New("offline cache request failed"))
			return nil
		})
		transactionComplete = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish(nil)
			return nil
		})
		transactionFailure = js.FuncOf(func(this js.Value, args []js.Value) any {
			finish(errors.New("offline cache transaction failed"))
			return nil
		})
		request.Set("onsuccess", requestSuccess)
		request.Set("onerror", requestFailure)
		tx.Set("oncomplete", transactionComplete)
		tx.Set("onabort", transactionFailure)
		tx.Set("onerror", transactionFailure)
		return nil
	})
	onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		result <- struct {
			data []byte
			err  error
		}{err: errors.New("offline database could not be opened")}
		return nil
	})
	open.Set("onupgradeneeded", onUpgrade)
	open.Set("onsuccess", onOpen)
	open.Set("onerror", onError)
	value := <-result
	cleanup()
	return value.data, value.err
}
