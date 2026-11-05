package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
)

const maxIngestBody = 2 << 20 // 2 MiB; manifests keep normal pushes far below this.

func (s *Server) handleIngestManifest(w http.ResponseWriter, r *http.Request) {
	cred, ok := s.pluginCredential(w, r)
	if !ok {
		return
	}
	manifest, err := s.opts.Ingest.ManifestFor(r.Context(), cred)
	if err != nil {
		s.writeIngestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleIngestPush(w http.ResponseWriter, r *http.Request) {
	cred, ok := s.pluginCredential(w, r)
	if !ok {
		return
	}
	raw, err := readJSONBody(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var push coreingest.Push
	if err = decodeStrictJSON(raw, &push); err != nil {
		writeError(w, http.StatusBadRequest, "invalid ingest JSON: "+err.Error())
		return
	}
	if err = push.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response, err := s.opts.Ingest.AcceptFor(r.Context(), cred, remoteIP(r.RemoteAddr), raw, push)
	if err != nil {
		if errors.Is(err, storeingest.ErrStaleSequence) || errors.Is(err, storeingest.ErrStaleManifest) {
			writeJSON(w, http.StatusConflict, response)
			return
		}
		s.writeIngestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type createIngestTokenRequest struct {
	AccountID string                  `json:"account_id"`
	Label     string                  `json:"label,omitempty"`
	Scopes    []coreingest.TokenScope `json:"scopes,omitempty"`
}

func (s *Server) handleCreateIngestToken(w http.ResponseWriter, r *http.Request) {
	raw, err := readJSONBody(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var request createIngestTokenRequest
	if err = decodeStrictJSON(raw, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid token request: "+err.Error())
		return
	}
	if strings.TrimSpace(request.AccountID) == "" {
		writeError(w, http.StatusBadRequest, "account_id is required")
		return
	}
	created, err := s.opts.Ingest.CreateToken(r.Context(), request.AccountID, request.Label, request.Scopes)
	if err != nil {
		s.writeIngestError(w, err)
		return
	}
	// The plaintext is returned only once and must not be cached by a browser
	// or intermediary. Request logging intentionally records no response body.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) writeIngestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storeingest.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", `Bearer realm="ingest"`)
		writeError(w, http.StatusUnauthorized, "invalid or revoked ingest token")
	case errors.Is(err, storeingest.ErrScopeDenied):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, storeingest.ErrInvalidPayload):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, storeingest.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, storeingest.ErrAccountAmbiguous):
		// 422, not 409: the plugin reads 409 as a stale sequence.
		writeError(w, http.StatusUnprocessableEntity, "several dev accounts: set the plugin's account (Conta)")
	default:
		s.log.Error("ingest request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "ingest failed")
	}
}

func bearerToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="ingest"`)
		writeError(w, http.StatusUnauthorized, "bearer ingest token required")
		return "", false
	}
	return parts[1], true
}

func readJSONBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBody)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, fmt.Errorf("request body exceeds %d bytes", maxIngestBody)
		}
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("request body is required")
	}
	return raw, nil
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
