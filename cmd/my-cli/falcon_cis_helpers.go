package main

import (
	"encoding/json"
	"fmt"
)

// jsonMarshalIndent is a thin wrapper so command files do not each import
// encoding/json for a single call.
func jsonMarshalIndent(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	return data, nil
}
