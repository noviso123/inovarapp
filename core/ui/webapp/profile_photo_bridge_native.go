//go:build !js || !wasm

package webapp

type customerProfilePhotoInput struct {
	DataURL string
	Error   string
}

func startCustomerProfilePhotoPicker() <-chan *customerProfilePhotoInput {
	out := make(chan *customerProfilePhotoInput, 1)
	out <- nil
	return out
}
