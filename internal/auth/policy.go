// Package auth holds the web login of AUTH_MODE=oidc (docs/04 sections 2.5
// and 8): the OIDC authorization code flow with PKCE against the issuer
// (Zitadel), the encrypted cookie that carries the flow state between
// /auth/login and /auth/callback, and the authorization policy. It opens no
// database: users and sessions are persisted by the caller.
package auth

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
)

// Zitadel role claims: the roles of the requesting project, and the same per
// project id. Both map a role key to {orgId: orgDomain}.
const (
	zitadelRolesClaim   = "urn:zitadel:iam:org:project:roles"
	zitadelProjectClaim = "urn:zitadel:iam:org:project:"
	zitadelRolesSuffix  = ":roles"
)

// Policy decides who may log in. With only a subject list, a listed subject
// passes; with only a required role, a holder of that project role passes;
// with both, a login needs both (owner decision of 2026-11-05). An empty
// policy allows nobody (config refuses it).
type Policy struct {
	AllowedSubjects []string
	RequiredRole    string
}

// Allows reports whether a user with this subject and these project roles is
// authorized.
func (p Policy) Allows(subject string, roles []string) bool {
	if subject == "" || (len(p.AllowedSubjects) == 0 && p.RequiredRole == "") {
		return false
	}
	if len(p.AllowedSubjects) > 0 && !slices.Contains(p.AllowedSubjects, subject) {
		return false
	}
	return p.RequiredRole == "" || slices.Contains(roles, p.RequiredRole)
}

// RolesFromClaims collects the role keys of the Zitadel project role claims,
// sorted and without duplicates: urn:zitadel:iam:org:project:roles (the
// project of the requesting client) and urn:zitadel:iam:org:project:<id>:roles.
// With projectID set, only that project's scoped claim counts; empty accepts
// any project id. Claims of any other shape are ignored.
func RolesFromClaims(claims map[string]json.RawMessage, projectID string) []string {
	seen := map[string]bool{}
	for name, raw := range claims {
		if name != zitadelRolesClaim && !projectClaim(name, projectID) {
			continue
		}
		var roles map[string]json.RawMessage
		if json.Unmarshal(raw, &roles) != nil {
			continue
		}
		for role := range roles {
			if role != "" {
				seen[role] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// projectClaim reports whether name is a project-scoped role claim, of
// projectID when it is set.
func projectClaim(name, projectID string) bool {
	id, ok := strings.CutPrefix(name, zitadelProjectClaim)
	if !ok {
		return false
	}
	id, ok = strings.CutSuffix(id, zitadelRolesSuffix)
	if !ok || id == "" {
		return false
	}
	return projectID == "" || id == projectID
}
