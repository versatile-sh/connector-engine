// Package args reads the JSON arguments of a call once and refuses an object that repeats a key.
package args

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Parse accepts absent, null or one JSON object whose keys are unique at every depth, and refuses anything else, trailing data included.
func Parse(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	first, err := d.Token()
	if err != nil {
		return notJSON(err)
	}
	if first != nil && first != json.Delim('{') {
		return errors.New("arguments must be a JSON object")
	}
	if first != nil {
		if err := object(d); err != nil {
			return err
		}
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("arguments carry data after the first JSON value")
	}
	return nil
}

func notJSON(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("arguments are not JSON: %v", err)
}

func object(d *json.Decoder) error {
	seen := map[string]bool{}
	for d.More() {
		k, err := d.Token()
		if err != nil {
			return notJSON(err)
		}
		key := k.(string)
		if seen[key] {
			return fmt.Errorf("arguments repeat the key %q", key)
		}
		seen[key] = true
		if err := value(d); err != nil {
			return err
		}
	}
	_, err := d.Token()
	return notJSON(err)
}

func value(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return notJSON(err)
	}
	switch t {
	case json.Delim('{'):
		return object(d)
	case json.Delim('['):
		for d.More() {
			if err := value(d); err != nil {
				return err
			}
		}
		_, err := d.Token()
		return notJSON(err)
	}
	return nil
}
