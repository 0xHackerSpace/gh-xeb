package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

// encodeJSON writes an indented JSON document, which is what every --json flag
// on this tree produces. doctor has its own writer because its payload is a
// pinned contract with a fixed shape; everything else comes through here.
func encodeJSON(w io.Writer, payload interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}
