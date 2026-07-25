package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const maxJSONBodyBytes = 1 << 20

func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return io.EOF
	}

	r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return mapJSONDecodeError(err)
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return multipleJSONValuesError()
	}

	return nil
}

func mapJSONDecodeError(err error) error {
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &syntaxErr):
		return malformedJSONError(syntaxErr.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return &SyntaxError{Message: "Request body contains malformed JSON"}
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			field = "body"
		}

		return invalidJSONTypeError(field)
	}

	var invalidUnmarshalErr *json.InvalidUnmarshalError
	if errors.As(err, &invalidUnmarshalErr) {
		panic(err)
	}

	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		field = strings.Trim(field, "\"")
		field = emptyJSONFieldName(field)
		if field == "" {
			field = "body"
		}

		return invalidJSONFieldError(field)
	}

	if strings.HasPrefix(err.Error(), "http: request body too large") {
		return bodyTooLargeError(maxJSONBodyBytes)
	}

	return err
}
