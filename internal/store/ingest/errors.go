// Package ingest persists authenticated plugin pushes using the pure wire
// contract in internal/core/ingest.
package ingest

import "errors"

var (
	ErrUnauthorized    = errors.New("ingest token is missing, revoked, or inactive")
	ErrInvalidPayload  = errors.New("invalid ingest payload")
	ErrStaleSequence   = errors.New("client sequence was already accepted")
	ErrStaleManifest   = errors.New("client must refresh the manifest and send a full push")
	ErrScopeDenied     = errors.New("ingest token does not allow the submitted item data")
	ErrAccountNotFound = errors.New("ingest account does not exist or is inactive")
)
