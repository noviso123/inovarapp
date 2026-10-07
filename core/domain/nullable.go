package domain

import (
	"bytes"
	"encoding/json"
)

// NullableString distinguishes an omitted optional field from a field present
// with JSON null, which the existing client serializes for lifecycle timestamps.
type NullableString struct {
	Value   *string
	Present bool
}

func (value NullableString) IsZero() bool { return !value.Present }

func (value NullableString) MarshalJSON() ([]byte, error) {
	if value.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*value.Value)
}

func (value *NullableString) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil
		return nil
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded
	return nil
}

func StringValue(value string) NullableString {
	copy := value
	return NullableString{Value: &copy, Present: true}
}

func NullString() NullableString {
	return NullableString{Present: true}
}
