package httpapi

import "net/http"

type DataResponse[T any] struct {
	Data T `json:"data"`
}

func WriteData[T any](w http.ResponseWriter, status int, data T) {
	writeJSON(w, status, DataResponse[T]{Data: data})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncoder(w, payload)
}
