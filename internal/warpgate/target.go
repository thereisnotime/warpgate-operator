package warpgate

import (
	"encoding/json"
	"fmt"
)

// TLS configuration for targets.
type TLSConfig struct {
	Mode   string `json:"mode"`   // Disabled, Preferred, Required
	Verify bool   `json:"verify"`
}

// SSH target options.
type SSHOptions struct {
	Kind              string  `json:"kind"` // always "Ssh"
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	Username          string  `json:"username"`
	AllowInsecureAlgos bool   `json:"allow_insecure_algos,omitempty"`
	Auth              SSHAuth `json:"auth"`
}

type SSHAuth struct {
	Kind     string `json:"kind"` // "Password" or "PublicKey"
	Password string `json:"password,omitempty"`
}

// HTTP target options.
type HTTPOptions struct {
	Kind         string            `json:"kind"` // always "Http"
	URL          string            `json:"url"`
	TLS          *TLSConfig        `json:"tls,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	ExternalHost string            `json:"external_host,omitempty"`
}

// MySQL target options.
type MySQLOptions struct {
	Kind     string     `json:"kind"` // always "MySql"
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Username string     `json:"username"`
	Password string     `json:"password,omitempty"`
	TLS      *TLSConfig `json:"tls,omitempty"`
}

// PostgreSQL target options.
type PostgresOptions struct {
	Kind     string     `json:"kind"` // always "Postgres"
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Username string     `json:"username"`
	Password string     `json:"password,omitempty"`
	TLS      *TLSConfig `json:"tls,omitempty"`
}

// Kubernetes target options.
type KubernetesOptions struct {
	Kind       string          `json:"kind"` // always "Kubernetes"
	ClusterURL string          `json:"cluster_url"`
	TLS        *TLSConfig      `json:"tls,omitempty"`
	Auth       KubernetesAuth  `json:"auth"`
}

type KubernetesAuth struct {
	Kind        string `json:"kind"` // "Token" or "Certificate"
	Token       string `json:"token,omitempty"`
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"private_key,omitempty"`
}

// Target represents a Warpgate target.
type Target struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	GroupID     string   `json:"group_id,omitempty"`
	AllowRoles  []string `json:"allow_roles,omitempty"`
	Options     json.RawMessage `json:"options"`
}

// TargetRequest is used for create/update operations.
type TargetRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	GroupID     string          `json:"group_id,omitempty"`
	Options     json.RawMessage `json:"options"`
}

// MarshalOptions marshals a typed options struct into json.RawMessage for use in TargetRequest.
func MarshalOptions(opts any) (json.RawMessage, error) {
	data, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshaling target options: %w", err)
	}
	return data, nil
}

// ParseOptionsKind extracts the "kind" field from raw options JSON.
func ParseOptionsKind(raw json.RawMessage) (string, error) {
	var k struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &k); err != nil {
		return "", fmt.Errorf("parsing options kind: %w", err)
	}
	return k.Kind, nil
}

func (c *Client) CreateTarget(req TargetRequest) (*Target, error) {
	var target Target
	if err := c.Post("/targets", req, &target); err != nil {
		return nil, err
	}
	return &target, nil
}

func (c *Client) GetTarget(id string) (*Target, error) {
	var target Target
	if err := c.Get(fmt.Sprintf("/targets/%s", id), &target); err != nil {
		return nil, err
	}
	return &target, nil
}

func (c *Client) GetTargetByName(name string) (*Target, error) {
	targets, err := c.ListTargets(name)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if t.Name == name {
			return &t, nil
		}
	}
	return nil, &APIError{StatusCode: 404, Body: fmt.Sprintf("target %q not found", name)}
}

func (c *Client) UpdateTarget(id string, req TargetRequest) (*Target, error) {
	var target Target
	if err := c.Put(fmt.Sprintf("/targets/%s", id), req, &target); err != nil {
		return nil, err
	}
	return &target, nil
}

func (c *Client) DeleteTarget(id string) error {
	return c.Delete(fmt.Sprintf("/targets/%s", id))
}

func (c *Client) ListTargets(search string) ([]Target, error) {
	path := "/targets"
	if search != "" {
		path += "?search=" + search
	}
	var targets []Target
	if err := c.Get(path, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}
