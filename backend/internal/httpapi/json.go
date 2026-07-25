package httpapi

import (
	"encoding/json"
	"io"
)

func jsonEncoder(w io.Writer, payload any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(payload)
}
