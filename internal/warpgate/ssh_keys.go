package warpgate

// SSHKey is a stored SSH client key that Warpgate uses to authenticate to targets.
type SSHKey struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Kind      string `json:"kind"` // Ed25519 or Rsa
	PublicKey string `json:"public_key"`
	IsDefault bool   `json:"is_default"`
}

func (c *Client) GetSSHOwnKeys() ([]SSHKey, error) {
	var keys []SSHKey
	if err := c.Get("/ssh/own-keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
