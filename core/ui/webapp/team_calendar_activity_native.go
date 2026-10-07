//go:build !js || !wasm

package webapp

func watchTeamCalendarActivity(func()) func() { return func() {} }
