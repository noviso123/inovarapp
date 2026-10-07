//go:build js && wasm

package webapp

import (
	"strings"
	"syscall/js"
)

func deviceContactPickerAvailable() bool {
	if picker := js.Global().Get("inovarPickDeviceContact"); picker.Type() == js.TypeFunction {
		return true
	}
	navigator := js.Global().Get("navigator")
	if navigator.Type() == js.TypeObject {
		contacts := navigator.Get("contacts")
		return contacts.Type() == js.TypeObject && contacts.Get("select").Type() == js.TypeFunction
	}
	return false
}

func pickDeviceContacts() <-chan deviceContactResult {
	out := make(chan deviceContactResult, 1)
	finished := false
	var resolve, reject js.Func
	finish := func(result deviceContactResult) {
		if finished {
			return
		}
		finished = true
		out <- result
		resolve.Release()
		reject.Release()
	}
	resolve = js.FuncOf(func(this js.Value, args []js.Value) any {
		contacts := make([]deviceContact, 0, 1)
		if len(args) > 0 {
			value := args[0]
			if value.Type() == js.TypeObject && js.Global().Get("Array").Call("isArray", value).Bool() {
				for i := 0; i < value.Get("length").Int(); i++ {
					contacts = append(contacts, decodeDeviceContact(value.Index(i)))
				}
			} else if value.Type() == js.TypeObject {
				contacts = append(contacts, decodeDeviceContact(value))
			}
		}
		finish(deviceContactResult{Contacts: contacts})
		return nil
	})
	reject = js.FuncOf(func(this js.Value, args []js.Value) any {
		code, message := "", ""
		if len(args) > 0 && args[0].Type() == js.TypeObject {
			if args[0].Get("code").Type() == js.TypeString {
				code = args[0].Get("code").String()
			}
			if args[0].Get("message").Type() == js.TypeString {
				message = args[0].Get("message").String()
			}
			if message == "" && args[0].Get("name").Type() == js.TypeString {
				message = args[0].Get("name").String()
			}
		}
		finish(classifyDeviceContactPickerError(code, message))
		return nil
	})
	var promise js.Value
	if picker := js.Global().Get("inovarPickDeviceContact"); picker.Type() == js.TypeFunction {
		promise = picker.Invoke()
	} else {
		contacts := js.Global().Get("navigator").Get("contacts")
		fields := js.ValueOf([]any{"name", "tel", "email"})
		options := js.ValueOf(map[string]any{"multiple": true})
		promise = contacts.Call("select", fields, options)
	}
	promise.Call("then", resolve).Call("catch", reject)
	return out
}

func decodeDeviceContact(value js.Value) deviceContact {
	contact := deviceContact{}
	if value.Type() != js.TypeObject {
		return contact
	}
	contact.Name = firstContactString(value, "displayName")
	if contact.Name == "" {
		name := value.Get("name")
		contact.Name = firstContactString(name, "formatted")
		if contact.Name == "" {
			contact.Name = strings.TrimSpace(firstContactString(name, "givenName") + " " + firstContactString(name, "familyName"))
		}
	}
	if contact.Name == "" {
		contact.Name = firstContactArrayString(value, "name")
	}
	contact.Phone = firstContactArrayField(value, "phoneNumbers", "value")
	if contact.Phone == "" {
		contact.Phone = firstContactArrayString(value, "tel")
	}
	contact.Email = firstContactArrayField(value, "emails", "value")
	if contact.Email == "" {
		contact.Email = firstContactArrayString(value, "email")
	}
	contact.Address = firstContactArrayField(value, "addresses", "streetAddress")
	if contact.Address == "" {
		contact.Address = firstContactArrayField(value, "addresses", "formatted")
	}
	contact.PostalCode = firstContactArrayField(value, "addresses", "postalCode")
	contact.City = firstContactArrayField(value, "addresses", "locality")
	contact.State = firstContactArrayField(value, "addresses", "region")
	return contact
}

func firstContactString(value js.Value, key string) string {
	if value.Type() != js.TypeObject {
		return ""
	}
	field := value.Get(key)
	if field.Type() == js.TypeString {
		return strings.TrimSpace(field.String())
	}
	return ""
}

func firstContactArrayString(value js.Value, key string) string {
	if value.Type() != js.TypeObject {
		return ""
	}
	items := value.Get(key)
	if items.Type() != js.TypeObject || items.Get("length").Type() != js.TypeNumber || items.Get("length").Int() == 0 {
		return ""
	}
	first := items.Index(0)
	if first.Type() == js.TypeString {
		return strings.TrimSpace(first.String())
	}
	return ""
}

func firstContactArrayField(value js.Value, key, field string) string {
	if value.Type() != js.TypeObject {
		return ""
	}
	items := value.Get(key)
	if items.Type() != js.TypeObject || items.Get("length").Type() != js.TypeNumber || items.Get("length").Int() == 0 {
		return ""
	}
	fallback := ""
	for i := 0; i < items.Get("length").Int(); i++ {
		item := items.Index(i)
		candidate := firstContactString(item, field)
		if candidate == "" {
			continue
		}
		if fallback == "" {
			fallback = candidate
		}
		preferred := item.Get("pref")
		if preferred.Type() == js.TypeBoolean && preferred.Bool() {
			return candidate
		}
	}
	return fallback
}
