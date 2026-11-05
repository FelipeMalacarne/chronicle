package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/FelipeMalacarne/ironman-planner/internal/auth"
	"github.com/FelipeMalacarne/ironman-planner/internal/auth/authtest"
)

func TestPolicyAllows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy auth.Policy
		sub    string
		roles  []string
		want   bool
	}{
		{"listed subject", auth.Policy{AllowedSubjects: []string{"1", "2"}}, "2", nil, true},
		{"unlisted subject", auth.Policy{AllowedSubjects: []string{"1"}}, "3", []string{"owner"}, false},
		{"required role", auth.Policy{RequiredRole: "owner"}, "3", []string{"member", "owner"}, true},
		{"missing role", auth.Policy{RequiredRole: "owner"}, "3", []string{"member"}, false},
		{"both set: role without a listed subject", auth.Policy{AllowedSubjects: []string{"1"}, RequiredRole: "owner"}, "3", []string{"owner"}, false},
		{"both set: listed subject without the role", auth.Policy{AllowedSubjects: []string{"1"}, RequiredRole: "owner"}, "1", []string{"member"}, false},
		{"both set: listed subject with the role", auth.Policy{AllowedSubjects: []string{"1"}, RequiredRole: "owner"}, "1", []string{"owner"}, true},
		{"empty policy", auth.Policy{}, "1", []string{"owner"}, false},
		{"empty subject", auth.Policy{AllowedSubjects: []string{""}}, "", nil, false},
	} {
		if got := tc.policy.Allows(tc.sub, tc.roles); got != tc.want {
			t.Errorf("%s: Allows = %v", tc.name, got)
		}
	}
}

func TestRolesFromClaims(t *testing.T) {
	var claims map[string]json.RawMessage
	raw := `{
		"sub": "1",
		"urn:zitadel:iam:org:project:roles": {"owner": {"o1": "d"}},
		"urn:zitadel:iam:org:project:999:roles": {"member": {"o1": "d"}, "owner": {"o1": "d"}},
		"urn:zitadel:iam:org:project:999:other": {"admin": {}},
		"urn:zitadel:iam:org:project:1:roles": ["not", "an", "object"],
		"roles": {"admin": {}}
	}`
	if err := json.Unmarshal([]byte(raw), &claims); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(auth.RolesFromClaims(claims, ""), ","); got != "member,owner" {
		t.Errorf("roles = %q", got)
	}
	// Pinned to project 777: the scoped claim of 999 no longer counts, the
	// requesting project's claim still does.
	claims["urn:zitadel:iam:org:project:777:roles"] = json.RawMessage(`{"viewer": {"o1": "d"}}`)
	if got := strings.Join(auth.RolesFromClaims(claims, "777"), ","); got != "owner,viewer" {
		t.Errorf("pinned roles = %q", got)
	}
}

func TestSealerRoundTrip(t *testing.T) {
	s, err := auth.NewSealer("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal(auth.Flow{State: "s", Nonce: "n", Verifier: "v", ReturnTo: "/plan"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(sealed)
	if err != nil || f.State != "s" || f.Nonce != "n" || f.Verifier != "v" || f.ReturnTo != "/plan" {
		t.Fatalf("open = %+v, %v", f, err)
	}
	other, _ := auth.NewSealer("another-secret-another-secret-xx")
	if _, err := other.Open(sealed); !errors.Is(err, auth.ErrBadFlow) {
		t.Errorf("other key: %v", err)
	}
	tampered := []byte(sealed)
	tampered[len(tampered)-2] ^= 1
	if _, err := s.Open(string(tampered)); !errors.Is(err, auth.ErrBadFlow) {
		t.Errorf("tampered: %v", err)
	}
	if _, err := s.Open("garbage!"); !errors.Is(err, auth.ErrBadFlow) {
		t.Errorf("garbage: %v", err)
	}
}

func TestSafeReturnPath(t *testing.T) {
	for raw, want := range map[string]string{
		"":                     "/",
		"/":                    "/",
		"/rotas/oziris-main":   "/rotas/oziris-main",
		"/plan?account=x#top":  "/plan?account=x#top",
		"//evil.example/":      "/",
		"/\\evil.example":      "/",
		"https://evil.example": "/",
		"plan":                 "/",
		"/auth/login":          "/",
		"/auth":                "/",
		"/authors":             "/authors",
	} {
		if got := auth.SafeReturnPath(raw); got != want {
			t.Errorf("SafeReturnPath(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestProviderExchange(t *testing.T) {
	ctx := context.Background()
	fake := authtest.New(t, "client-1")
	p, err := auth.NewProvider(ctx, auth.Config{Issuer: fake.Issuer(), ClientID: "client-1", RedirectURL: "http://app/auth/callback"})
	if err != nil {
		t.Fatal(err)
	}
	login := func(u authtest.User) (auth.Identity, string, error) {
		verifier := oauth2.GenerateVerifier()
		back, authErr := fake.Authorize(p.AuthCodeURL("state-1", "nonce-1", verifier), u)
		if authErr != nil {
			t.Fatal(authErr)
		}
		bu, _ := url.Parse(back)
		if bu.Query().Get("state") != "state-1" {
			t.Fatalf("state = %q", bu.Query().Get("state"))
		}
		return p.Exchange(ctx, bu.Query().Get("code"), verifier, "nonce-1")
	}
	id, raw, err := login(authtest.User{Subject: "42", Username: "felipe", Email: "f@example.org", Name: "Felipe", Roles: []string{"owner"}, ProjectID: "77"})
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "42" || id.Issuer != fake.Issuer() || id.Username != "felipe" || id.Email != "f@example.org" ||
		id.DisplayName != "Felipe" || strings.Join(id.Roles, ",") != "owner" || raw == "" {
		t.Errorf("identity = %+v", id)
	}

	// Roles only in userinfo ("Assert roles on authentication" without the
	// ID token toggle).
	id, _, err = login(authtest.User{Subject: "43", Roles: []string{"owner"}, RolesInUserinfo: true})
	if err != nil || strings.Join(id.Roles, ",") != "owner" {
		t.Errorf("userinfo roles = %+v, %v", id, err)
	}
	id, _, err = login(authtest.User{Subject: "44", Username: "u44", Email: "u@example.org", Name: "U", Roles: []string{"owner"}, ProfileInUserinfo: true})
	if err != nil || id.Username != "u44" || id.Email != "u@example.org" || id.DisplayName != "U" || strings.Join(id.Roles, ",") != "owner" {
		t.Errorf("userinfo profile = %+v, %v", id, err)
	}

	fake.WrongNonce = true
	if _, _, err := login(authtest.User{Subject: "42"}); !errors.Is(err, auth.ErrNonce) {
		t.Errorf("wrong nonce: %v", err)
	}
	fake.WrongNonce = false

	// A wrong PKCE verifier fails at the token endpoint.
	back, _ := fake.Authorize(p.AuthCodeURL("s", "n", oauth2.GenerateVerifier()), authtest.User{Subject: "42"})
	bu, _ := url.Parse(back)
	if _, _, err := p.Exchange(ctx, bu.Query().Get("code"), oauth2.GenerateVerifier(), "n"); err == nil {
		t.Error("wrong verifier accepted")
	}

	end, _ := url.Parse(p.EndSessionURL("tok", "http://app/"))
	if !strings.HasPrefix(end.String(), fake.EndSessionEndpoint()) || end.Query().Get("id_token_hint") != "tok" ||
		end.Query().Get("client_id") != "client-1" || end.Query().Get("post_logout_redirect_uri") != "http://app/" {
		t.Errorf("end session = %s", end)
	}
}
