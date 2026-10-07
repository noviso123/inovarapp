//go:build !js || !wasm

package webapp

func startAppliancePhotoPicker() <-chan []appliancePhotoInput {
	return startAppliancePhotoPickerLimit(0)
}

func startAppliancePhotoPickerLimit(int) <-chan []appliancePhotoInput {
	out := make(chan []appliancePhotoInput, 1)
	out <- nil
	return out
}
