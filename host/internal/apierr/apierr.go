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

// maxBody is the largest accepted JSON request body.
const maxBody = 64 << 10

type Error struct {
	Status  int    `json:"-"`
	Op      string `json:"op"`
	Tab     string `json:"tab,omitempty"`
	Message string `json:"message"`
	Kind    string `json:"kind"`
	Hint    string `json:"hint,omitempty"`
}

func BadRequest(op, tab, msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Op: op, Tab: tab, Kind: KindBadRequest, Message: msg}
}

func NotFound(op, tab, msg string) *Error {
	return &Error{Status: http.StatusNotFound, Op: op, Tab: tab, Kind: KindNotFound, Message: msg}
}

func Unauthorized(op, msg string) *Error {
	return &Error{Status: http.StatusUnauthorized, Op: op, Kind: KindUnauthorized, Message: msg}
}

func Internal(op, tab, msg string) *Error {
	return &Error{Status: http.StatusInternalServerError, Op: op, Tab: tab, Kind: KindInternal, Message: msg}
}

// HandlerFunc is an endpoint that returns its error instead of writing it. It returns non-nil only before writing anything.
type HandlerFunc func(http.ResponseWriter, *http.Request) *Error

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if e := f(w, r); e != nil {
		Write(w, e)
	}
}

// Write sends e as {"error": e} with status e.Status.
func Write(w http.ResponseWriter, e *Error) {
	WriteJSON(w, e.Status, struct {
		Error *Error `json:"error"`
	}{e})
}

// DecodeJSON reads exactly one JSON object with only known fields into v.
// Bodies over maxBody give 413; anything else malformed gives 400.
func DecodeJSON(w http.ResponseWriter, r *http.Request, op string, v any) *Error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return &Error{Status: http.StatusRequestEntityTooLarge, Op: op, Kind: KindBadRequest,
				Message: fmt.Sprintf("request body: expected at most %d bytes, got more", maxBody)}
		}
		return BadRequest(op, "", fmt.Sprintf("request body: expected one JSON object with known fields, got %v", err))
	}
	if dec.More() {
		return BadRequest(op, "", "request body: expected one JSON object, got trailing data")
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
