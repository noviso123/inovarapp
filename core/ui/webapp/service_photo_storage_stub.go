//go:build !js || !wasm

package webapp

func saveLocalServicePhoto(serviceID string, input appliancePhotoInput) error {
	return errLocalPhotoStorageUnavailable
}

func listLocalServicePhotos(serviceID string) ([]localServicePhoto, error) {
	return nil, errLocalPhotoStorageUnavailable
}

func deleteLocalServicePhoto(id string) error {
	return errLocalPhotoStorageUnavailable
}
