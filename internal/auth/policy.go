// Package auth holds the web login of AUTH_MODE=oidc (docs/04 sections 2.5
// and 8): the OIDC authorization code flow with PKCE against the issuer
// (Zitadel), the encrypted cookie that carries the flow state between
// /auth/login and /auth/callback, and the authorization policy. It opens no
// database: users and sessions are persisted by the caller.
package auth

import (
	"encoding/json"
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

// Policy decides who may log in: a listed subject OR a holder of the
// required project role. An empty policy allows nobody (config refuses it).
type Policy struct {
	AllowedSubjects []string
	RequiredRole    string
}

// Allows reports whether a user with this subject and these project roles is
// authorized.
func (p Policy) Allows(subject string, roles []string) bool {
	if subject == "" {
		return false
	}
	for _, s := range p.AllowedSubjects {
		if s == subject {
			return true
		}
	}
	if p.RequiredRole == "" {
		return false
	}
	for _, r := range roles {
		if r == p.RequiredRole {
			return true
		}
	}
	return false
}

// RolesFromClaims collects the role keys of every Zitadel project role claim
// (urn:zitadel:iam:org:project:roles and urn:zitadel:iam:org:project:<id>:roles),
// sorted and without duplicates. Claims of any other shape are ignored.
func RolesFromClaims(claims map[string]json.RawMessage) []string {
	seen := map[string]bool{}
	for name, raw := range claims {
		if name != zitadelRolesClaim && (!strings.HasPrefix(name, zitadelProjectClaim) || !strings.HasSuffix(name, zitadelRolesSuffix)) {
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
