package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexString accepts string, number, bool, or null JSON values and stores them as a string.
type FlexString string

// UnmarshalJSON implements json.Unmarshaler.
func (f *FlexString) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(trimmed)
	return nil
}

// String returns the underlying Go string.
func (f FlexString) String() string {
	return string(f)
}

// FlexColor accepts unsigned integers or hex color strings ("0xFF2196F3", "#2196F3").
type FlexColor uint32

// UnmarshalJSON implements json.Unmarshaler.
func (c *FlexColor) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*c = 0
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			*c = 0
			return nil
		}
		s = strings.TrimPrefix(s, "0x")
		s = strings.TrimPrefix(s, "0X")
		s = strings.TrimPrefix(s, "#")
		if len(s) == 6 {
			s = "FF" + s
		}
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			n, decErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
			if decErr != nil {
				*c = 0
				return nil
			}
			*c = FlexColor(n)
			return nil
		}
		*c = FlexColor(v)
		return nil
	}
	var n uint64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*c = FlexColor(n)
	return nil
}

// StringSlice persists []string as a JSON array string across SQLite, MySQL, and PostgreSQL.
type StringSlice []string

// Scan implements sql.Scanner.
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	var raw string
	switch v := value.(type) {
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		return fmt.Errorf("unsupported StringSlice scan type %T", value)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		*s = []string{}
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		*s = []string{}
		return nil
	}
	*s = out
	return nil
}

// Value implements driver.Valuer.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}
