package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes v as a JSON response body with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a {"message": "..."} envelope with the given status code.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"message": message})
}

// Actor reads the X-Actor header, defaulting to "api" when absent, matching
// the NestJS reference implementation's @Headers('x-actor') actor = 'api'.
func Actor(r *http.Request) string {
	if v := r.Header.Get("X-Actor"); v != "" {
		return v
	}
	return "api"
}

// DecodeJSON decodes the request body into v. Missing/empty bodies decode as
// a zero-value v (mirroring Nest's optional @Body()).
func DecodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		if err.Error() == "EOF" {
			return nil
		}
		return err
	}
	return nil
}
