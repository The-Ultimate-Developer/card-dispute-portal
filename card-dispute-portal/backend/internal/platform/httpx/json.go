package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON sets the content type, status and encodes v.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
