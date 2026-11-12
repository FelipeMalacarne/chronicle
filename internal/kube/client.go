// Package kube is the minimal Kubernetes API client of the workers (docs/11,
// decision W1): Deployments by server-side apply, pods and TokenReview over
// plain REST, without client-go. It covers only what the reconciler and the
// worker token check need.
package kube

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// In-cluster service account files.
const (
	saDir         = "/var/run/secrets/kubernetes.io/serviceaccount"
	FieldManager  = "chronicle"
	contentApply  = "application/apply-patch+yaml"
	contentJSON   = "application/json"
	maxBodyAnswer = 4 << 20
)

// ErrNotFound is a 404 from the API.
var ErrNotFound = errors.New("kube: not found")

// APIError is a non-2xx answer of the API server.
type APIError struct {
	Status  int
	Reason  string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kube: HTTP %d %s: %s", e.Status, e.Reason, e.Message)
}

// Client talks to one API server in one namespace.
type Client struct {
	// Base is the API server, e.g. https://10.43.0.1:443.
	Base string
	// Namespace is where the workers live.
	Namespace string
	// Token gives the bearer of each request; the projected service account
	// token rotates, so InCluster rereads the file every time.
	Token func() (string, error)
	HTTP  *http.Client
}

// InCluster builds the client of a pod: the API server from
// KUBERNETES_SERVICE_HOST/PORT, the service account's CA and token, and its
// namespace unless namespace is set.
func InCluster(namespace string) (*Client, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("kube: not in a cluster (KUBERNETES_SERVICE_HOST is unset)")
	}
	ca, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("kube: service account CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("kube: service account CA has no certificate")
	}
	if namespace == "" {
		b, err := os.ReadFile(saDir + "/namespace")
		if err != nil {
			return nil, fmt.Errorf("kube: service account namespace: %w", err)
		}
		namespace = strings.TrimSpace(string(b))
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("kube: http.DefaultTransport is not an *http.Transport")
	}
	tr := base.Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &Client{
		Base:      "https://" + net.JoinHostPort(host, port),
		Namespace: namespace,
		Token:     TokenFile(saDir + "/token"),
		HTTP:      &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}, nil
}

// TokenFile reads a token file on every call.
func TokenFile(path string) func() (string, error) {
	return func() (string, error) {
		b, err := os.ReadFile(path) //nolint:gosec // a fixed service account path
		if err != nil {
			return "", fmt.Errorf("kube: token: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, contentType string, body, out any) error {
	u := strings.TrimRight(c.Base, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", contentJSON)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != nil {
		tok, terr := c.Token()
		if terr != nil {
			return terr
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("kube: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAnswer))
	if err != nil {
		return fmt.Errorf("kube: read answer: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var st struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &st)
		if st.Message == "" {
			st.Message = strings.TrimSpace(string(raw))
		}
		return &APIError{Status: resp.StatusCode, Reason: st.Reason, Message: st.Message}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("kube: decode answer: %w", err)
		}
	}
	return nil
}

func (c *Client) deployments() string {
	return "/apis/apps/v1/namespaces/" + url.PathEscape(c.Namespace) + "/deployments"
}

// ApplyDeployment creates or updates d by server-side apply (field manager
// "chronicle", forcing conflicts: the server owns the workers it creates).
func (c *Client) ApplyDeployment(ctx context.Context, d Deployment) error {
	q := url.Values{"fieldManager": {FieldManager}, "force": {"true"}}
	return c.do(ctx, http.MethodPatch, c.deployments()+"/"+url.PathEscape(d.Metadata.Name), q, contentApply, d, nil)
}

// DeleteDeployment deletes a Deployment and, in the background, its pods; a
// missing one is not an error.
func (c *Client) DeleteDeployment(ctx context.Context, name string) error {
	body := map[string]any{"kind": "DeleteOptions", "apiVersion": "v1", "propagationPolicy": "Background"}
	err := c.do(ctx, http.MethodDelete, c.deployments()+"/"+url.PathEscape(name), nil, contentJSON, body, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// ListDeployments lists the namespace's Deployments that match a label
// selector.
func (c *Client) ListDeployments(ctx context.Context, selector string) ([]Deployment, error) {
	var out struct {
		Items []Deployment `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, c.deployments(), url.Values{"labelSelector": {selector}}, "", nil, &out)
	return out.Items, err
}

// ListPods lists the namespace's pods that match a label selector.
func (c *Client) ListPods(ctx context.Context, selector string) ([]Pod, error) {
	var out struct {
		Items []Pod `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/namespaces/"+url.PathEscape(c.Namespace)+"/pods",
		url.Values{"labelSelector": {selector}}, "", nil, &out)
	return out.Items, err
}

// DeletePod deletes one pod (its Deployment makes a new one); a missing pod
// is not an error.
func (c *Client) DeletePod(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/api/v1/namespaces/"+url.PathEscape(c.Namespace)+"/pods/"+url.PathEscape(name), nil, "", nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// GetPod reads one pod of the namespace.
func (c *Client) GetPod(ctx context.Context, name string) (Pod, error) {
	var p Pod
	err := c.do(ctx, http.MethodGet, "/api/v1/namespaces/"+url.PathEscape(c.Namespace)+"/pods/"+url.PathEscape(name), nil, "", nil, &p)
	return p, err
}

// TokenReviewStatus is what the API server says about a token.
type TokenReviewStatus struct {
	Authenticated bool `json:"authenticated"`
	User          struct {
		Username string              `json:"username"`
		UID      string              `json:"uid"`
		Extra    map[string][]string `json:"extra"`
	} `json:"user"`
	Audiences []string `json:"audiences"`
	Error     string   `json:"error"`
}

// TokenReview asks the API server whether token is valid for the audiences
// (it needs system:auth-delegator).
func (c *Client) TokenReview(ctx context.Context, token string, audiences []string) (TokenReviewStatus, error) {
	body := map[string]any{
		"apiVersion": "authentication.k8s.io/v1",
		"kind":       "TokenReview",
		"spec":       map[string]any{"token": token, "audiences": audiences},
	}
	var out struct {
		Status TokenReviewStatus `json:"status"`
	}
	err := c.do(ctx, http.MethodPost, "/apis/authentication.k8s.io/v1/tokenreviews", nil, contentJSON, body, &out)
	return out.Status, err
}
