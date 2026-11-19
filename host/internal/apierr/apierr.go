// Package apierr is the JSON error body shared by every roosterm HTTP endpoint.
package apierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

const (
	KindBadRequest      = "bad_request"
	KindUnauthorized    = "unauthorized"
	KindNotFound        = "not_found"
	KindTooManyAttempts = "too_many_attempts"
	KindCoreOffline     = "core_offline"
	KindInternal        = "internal"
)

// MaxBody is the largest accepted JSON request body.
const MaxBody = 64 << 10

type Error struct {
	Status  int    `json:"-"`
	Op      string `json:"op"`
	Tab     string `json:"tab,omitempty"`
	Message string `json:"message"`
	Kind    string `json:"kind"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Error) Error() string {
	if e.Tab == "" {
		return fmt.Sprintf("%s: %s (kind %s)", e.Op, e.Message, e.Kind)
	}
	return fmt.Sprintf("%s %s: %s (kind %s)", e.Op, e.Tab, e.Message, e.Kind)
}

// Write sends e as {"error": e} with status e.Status.
func Write(w http.ResponseWriter, e *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	if err := json.NewEncoder(w).Encode(struct {
		Error *Error `json:"error"`
	}{e}); err != nil {
		log.Printf("write error body for %s: %v", e.Op, err)
	}
}

// DecodeJSON reads exactly one JSON object with only known fields into v.
// Bodies over MaxBody give 413; anything else malformed gives 400.
func DecodeJSON(w http.ResponseWriter, r *http.Request, op string, v any) *Error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return &Error{Status: http.StatusRequestEntityTooLarge, Op: op, Kind: KindBadRequest,
				Message: fmt.Sprintf("request body: expected at most %d bytes, got more", MaxBody)}
		}
		return &Error{Status: http.StatusBadRequest, Op: op, Kind: KindBadRequest,
			Message: fmt.Sprintf("request body: expected one JSON object with known fields, got %v", err)}
	}
	if dec.More() {
		return &Error{Status: http.StatusBadRequest, Op: op, Kind: KindBadRequest,
			Message: "request body: expected one JSON object, got trailing data"}
	}
	return nil
}

// WriteJSON sends v as a JSON body with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write JSON body (status %d): %v", status, err)
	}
}
