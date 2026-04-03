package warpgate

type SSHKey struct {
	Kind            string `json:"kind"`
	PublicKeyBase64 string `json:"public_key_base64"`
}

func (c *Client) GetSSHOwnKeys() ([]SSHKey, error) {
	var keys []SSHKey
	if err := c.Get("/ssh/own-keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
