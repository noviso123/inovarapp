package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNullableStringPreservesAbsentNullAndValueLifecycleStates(t *testing.T) {
	value := "2026-10-05T12:00:00Z"
	type lifecycle struct {
		CompletedAt NullableString `json:"completedAt,omitzero"`
	}
	for _, tc := range []struct {
		name string
		row  lifecycle
		want string
	}{
		{name: "absent", row: lifecycle{}, want: "{}"},
		{name: "explicit null", row: lifecycle{CompletedAt: NullString()}, want: `{"completedAt":null}`},
		{name: "value", row: lifecycle{CompletedAt: StringValue(value)}, want: `{"completedAt":"2026-10-05T12:00:00Z"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.row)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(encoded)) != tc.want {
				t.Fatalf("JSON = %s, want %s", encoded, tc.want)
			}
			var decoded lifecycle
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.CompletedAt.Present != tc.row.CompletedAt.Present || (tc.row.CompletedAt.Value == nil) != (decoded.CompletedAt.Value == nil) {
				t.Fatalf("round trip changed null state: %#v", decoded.CompletedAt)
			}
		})
	}
}
