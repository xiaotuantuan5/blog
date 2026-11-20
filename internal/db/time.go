package db

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// TimeLayout is how timestamps are stored: fixed-width UTC, so string
// comparison and ORDER BY match chronological order.
const TimeLayout = "2006-01-02T15:04:05.000Z"

// Time is a time.Time stored as TEXT in TimeLayout.
type Time struct{ time.Time }

// Now returns the current time truncated to milliseconds.
func Now() Time { return Time{time.Now().UTC().Truncate(time.Millisecond)} }

func (t Time) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.UTC().Format(TimeLayout), nil
}

func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	case time.Time:
		t.Time = v.UTC()
		return nil
	}
	return fmt.Errorf("db.Time: cannot scan %T", src)
}

func (t *Time) parse(s string) error {
	p, err := time.Parse(TimeLayout, s)
	if err != nil {
		return err
	}
	t.Time = p
	return nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.UTC().Format(time.RFC3339) + `"`), nil
}
