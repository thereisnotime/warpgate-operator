package warpgate

import (
	"encoding/json"
	"fmt"
)

// TLSConfig configures TLS for a target connection. Always sent: the API
// treats an absent block as "verify on", so the operator states it explicitly.
type TLSConfig struct {
	Mode   string `json:"mode"` // Disabled, Preferred, Required
	Verify bool   `json:"verify"`
}

// SSH target options.
type SSHOptions struct {
	Kind               string  `json:"kind"` // always "Ssh"
	Host               string  `json:"host"`
	Port               int     `json:"port"`
	Username           string  `json:"username"`
	AllowInsecureAlgos bool    `json:"allow_insecure_algos"`
	Auth               SSHAuth `json:"auth"`
	JumpHost           string  `json:"jump_host,omitempty"` // Warpgate target UUID
}

type SSHAuth struct {
	Kind     string `json:"kind"` // "Password", "PublicKey" or "IamRole"
	Password string `json:"password,omitempty"`
	KeyID    string `json:"key_id,omitempty"` // stored client key UUID; empty uses the default keys
}

// HTTP target options.
type HTTPOptions struct {
	Kind         string            `json:"kind"` // always "Http"
	URL          string            `json:"url"`
	TLS          TLSConfig         `json:"tls"`
	Headers      map[string]string `json:"headers"`
	ExternalHost string            `json:"external_host,omitempty"`
}

// DatabaseAuth is shared by MySQL and PostgreSQL targets.
type DatabaseAuth struct {
	Kind     string `json:"kind"` // "Password" or "IamRole"
	Password string `json:"password,omitempty"`
}

// MySQL target options.
type MySQLOptions struct {
	Kind                string       `json:"kind"` // always "MySql"
	Host                string       `json:"host"`
	Port                int          `json:"port"`
	Username            string       `json:"username"`
	Auth                DatabaseAuth `json:"auth"`
	TLS                 TLSConfig    `json:"tls"`
	DefaultDatabaseName string       `json:"default_database_name,omitempty"`
}

// PostgreSQL target options.
type PostgresOptions struct {
	Kind                string       `json:"kind"` // always "Postgres"
	Host                string       `json:"host"`
	Port                int          `json:"port"`
	Username            string       `json:"username"`
	Auth                DatabaseAuth `json:"auth"`
	TLS                 TLSConfig    `json:"tls"`
	ProtocolVersion     string       `json:"protocol_version"` // "3.0" or "3.2"
	IdleTimeout         string       `json:"idle_timeout,omitempty"`
	DefaultDatabaseName string       `json:"default_database_name,omitempty"`
}

// Kubernetes target options.
type KubernetesOptions struct {
	Kind       string         `json:"kind"` // always "Kubernetes"
	ClusterURL string         `json:"cluster_url"`
	TLS        TLSConfig      `json:"tls"`
	Auth       KubernetesAuth `json:"auth"`
}

type KubernetesAuth struct {
	Kind        string `json:"kind"` // "Token", "Certificate" or "IamRole"
	Token       string `json:"token,omitempty"`
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"private_key,omitempty"`
}

// RDP target options.
type RDPOptions struct {
	Kind             string       `json:"kind"` // always "Rdp"
	Host             string       `json:"host"`
	Port             int          `json:"port"`
	Username         string       `json:"username"`
	Domain           string       `json:"domain,omitempty"`
	Auth             PasswordAuth `json:"auth"`
	VerifyTLS        bool         `json:"verify_tls"`
	Compression      string       `json:"compression"` // "remotefx" or "lossless"
	InteractiveLogon bool         `json:"interactive_logon"`
	TLSSecurity      string       `json:"tls_security"` // Tls12, Tls12WithLegacyCiphers, Tls10Unsafe
}

// PasswordAuth is the only RDP auth kind and the password VNC auth kind.
type PasswordAuth struct {
	Kind     string `json:"kind"` // "Password"
	Password string `json:"password,omitempty"`
}

// VNC target options.
type VNCOptions struct {
	Kind string       `json:"kind"` // always "Vnc"
	Host string       `json:"host"`
	Port int          `json:"port"`
	Auth PasswordAuth `json:"auth"` // Kind "None" or "Password"
}

// Target represents a Warpgate target.
type Target struct {
	ID                       string          `json:"id,omitempty"`
	Name                     string          `json:"name"`
	Description              string          `json:"description,omitempty"`
	GroupID                  string          `json:"group_id,omitempty"`
	AllowRoles               []string        `json:"allow_roles,omitempty"`
	Options                  json.RawMessage `json:"options"`
	RateLimitBytesPerSecond  *int64          `json:"rate_limit_bytes_per_second,omitempty"`
	TicketMaxDurationSeconds *int64          `json:"ticket_max_duration_seconds,omitempty"`
	TicketRequestsDisabled   bool            `json:"ticket_requests_disabled"`
	TicketRequireApproval    bool            `json:"ticket_require_approval"`
	RequireApproval          bool            `json:"require_approval"`
	TicketMaxUses            *int64          `json:"ticket_max_uses,omitempty"`
}

// TargetRequest is used for create/update operations. The three access gates
// are required by the API on every write.
type TargetRequest struct {
	Name                     string          `json:"name"`
	Description              string          `json:"description,omitempty"`
	GroupID                  string          `json:"group_id,omitempty"`
	Options                  json.RawMessage `json:"options"`
	RateLimitBytesPerSecond  *int64          `json:"rate_limit_bytes_per_second,omitempty"`
	TicketMaxDurationSeconds *int64          `json:"ticket_max_duration_seconds,omitempty"`
	TicketRequestsDisabled   bool            `json:"ticket_requests_disabled"`
	TicketRequireApproval    bool            `json:"ticket_require_approval"`
	RequireApproval          bool            `json:"require_approval"`
	TicketMaxUses            *int64          `json:"ticket_max_uses,omitempty"`
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
