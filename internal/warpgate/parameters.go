package warpgate

type Parameters struct {
	AllowOwnCredentialManagement     bool `json:"allow_own_credential_management"`
	RateLimitBytesPerSecond          int  `json:"rate_limit_bytes_per_second,omitempty"`
	SSHClientAuthPublicKey           bool `json:"ssh_client_auth_publickey"`
	SSHClientAuthPassword            bool `json:"ssh_client_auth_password"`
	SSHClientAuthKeyboardInteractive bool `json:"ssh_client_auth_keyboard_interactive"`
	MinimizePasswordLogin            bool `json:"minimize_password_login"`
}

func (c *Client) GetParameters() (*Parameters, error) {
	var params Parameters
	if err := c.Get("/parameters", &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func (c *Client) UpdateParameters(params Parameters) error {
	// PUT returns 201 with no body — we don't parse a response.
	return c.Put("/parameters", params, nil)
}
