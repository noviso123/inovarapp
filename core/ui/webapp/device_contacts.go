package webapp

import "strings"

type deviceContact struct {
	Name       string
	Phone      string
	Email      string
	Address    string
	PostalCode string
	City       string
	State      string
}

type deviceContactResult struct {
	Contacts  []deviceContact
	Err       string
	Cancelled bool
}

func classifyDeviceContactPickerError(code, message string) deviceContactResult {
	lower := strings.ToLower(strings.TrimSpace(message))
	if code == "OS-PLUG-CONT-0006" || strings.Contains(lower, "cancel") || strings.Contains(lower, "abort") {
		return deviceContactResult{Cancelled: true}
	}
	if code == "OS-PLUG-CONT-0020" {
		return deviceContactResult{Err: "Contacts permission was denied."}
	}
	if message == "" {
		message = "Não foi possível acessar os contatos do dispositivo."
	}
	return deviceContactResult{Err: message}
}
