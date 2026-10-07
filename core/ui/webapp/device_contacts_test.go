package webapp

import (
	"strings"
	"testing"
)

func TestApplyDeviceContactNormalizesAndPreservesAccountEmail(t *testing.T) {
	form := newTeamCustomerForm()
	form.Email = "portal@example.com"
	applyDeviceContact(form, deviceContact{Name: "Ana Souza", Phone: " +55 (27) 99999-0000 ", Email: "agenda@example.com"})
	if form.Name != "Ana Souza" || form.Phone != "+5527999990000" || form.Email != "portal@example.com" {
		t.Fatalf("form after contact fill=%#v", form)
	}
	form.Email = ""
	applyDeviceContact(form, deviceContact{Name: "Ana Souza", Phone: "27999990000", Email: "agenda@example.com"})
	if form.Email != "agenda@example.com" {
		t.Fatalf("contact email=%q", form.Email)
	}
}

func TestApplyDeviceContactDoesNotSetHiddenEmailOnEdit(t *testing.T) {
	form := newTeamCustomerForm()
	form.Editing = true
	applyDeviceContact(form, deviceContact{Name: "Ana Souza", Phone: "+55 27 99999-0000", Email: "agenda@example.com"})
	if form.Name != "Ana Souza" || form.Phone != "+5527999990000" || form.Email != "" {
		t.Fatalf("edited form after contact fill=%#v", form)
	}
}

func TestClassifyDeviceContactPickerErrors(t *testing.T) {
	if result := classifyDeviceContactPickerError("OS-PLUG-CONT-0006", "The operation was cancelled."); !result.Cancelled || result.Err != "" {
		t.Fatalf("cancel result=%#v", result)
	}
	if result := classifyDeviceContactPickerError("", "AbortError"); !result.Cancelled || result.Err != "" {
		t.Fatalf("browser abort result=%#v", result)
	}
	if result := classifyDeviceContactPickerError("OS-PLUG-CONT-0020", "Contacts permission was denied."); result.Cancelled || !strings.Contains(result.Err, "permission") {
		t.Fatalf("permission result=%#v", result)
	}
	if result := classifyDeviceContactPickerError("", ""); result.Cancelled || result.Err == "" {
		t.Fatalf("empty error result=%#v", result)
	}
}
