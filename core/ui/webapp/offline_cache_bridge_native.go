//go:build !js || !wasm

package webapp

import "errors"

func saveOfflineSnapshot(offlineAccountSnapshot) error {
	return errors.New("offline cache is only available in the browser")
}

func loadOfflineSnapshot(string) (offlineAccountSnapshot, error) {
	return offlineAccountSnapshot{}, errors.New("offline cache is only available in the browser")
}

func deleteOfflineSnapshot(string) error { return nil }
