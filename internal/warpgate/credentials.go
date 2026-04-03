package warpgate

import "fmt"

// Password credential.

type PasswordCredential struct {
	ID       string `json:"id"`
	Password string `json:"password"`
}

func (c *Client) CreatePasswordCredential(userID, password string) (*PasswordCredential, error) {
	var cred PasswordCredential
	if err := c.Post(fmt.Sprintf("/users/%s/credentials/passwords", userID), map[string]string{"password": password}, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) DeletePasswordCredential(userID, credentialID string) error {
	return c.Delete(fmt.Sprintf("/users/%s/credentials/passwords/%s", userID, credentialID))
}

// Public key credential.

type PublicKeyCredential struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	OpenSSHPublicKey string `json:"openssh_public_key"`
	DateAdded        string `json:"date_added,omitempty"`
	LastUsed         string `json:"last_used,omitempty"`
}

type PublicKeyCredentialRequest struct {
	Label            string `json:"label"`
	OpenSSHPublicKey string `json:"openssh_public_key"`
}

func (c *Client) CreatePublicKeyCredential(userID string, req PublicKeyCredentialRequest) (*PublicKeyCredential, error) {
	var cred PublicKeyCredential
	if err := c.Post(fmt.Sprintf("/users/%s/credentials/public-keys", userID), req, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) ListPublicKeyCredentials(userID string) ([]PublicKeyCredential, error) {
	var creds []PublicKeyCredential
	if err := c.Get(fmt.Sprintf("/users/%s/credentials/public-keys", userID), &creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (c *Client) UpdatePublicKeyCredential(userID, credentialID string, req PublicKeyCredentialRequest) (*PublicKeyCredential, error) {
	var cred PublicKeyCredential
	if err := c.Put(fmt.Sprintf("/users/%s/credentials/public-keys/%s", userID, credentialID), req, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) DeletePublicKeyCredential(userID, credentialID string) error {
	return c.Delete(fmt.Sprintf("/users/%s/credentials/public-keys/%s", userID, credentialID))
}

// SSO credential.

type SsoCredential struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Email    string `json:"email"`
}

type SsoCredentialRequest struct {
	Provider string `json:"provider"`
	Email    string `json:"email"`
}

func (c *Client) CreateSsoCredential(userID string, req SsoCredentialRequest) (*SsoCredential, error) {
	var cred SsoCredential
	if err := c.Post(fmt.Sprintf("/users/%s/credentials/sso", userID), req, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) ListSsoCredentials(userID string) ([]SsoCredential, error) {
	var creds []SsoCredential
	if err := c.Get(fmt.Sprintf("/users/%s/credentials/sso", userID), &creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (c *Client) UpdateSsoCredential(userID, credentialID string, req SsoCredentialRequest) (*SsoCredential, error) {
	var cred SsoCredential
	if err := c.Put(fmt.Sprintf("/users/%s/credentials/sso/%s", userID, credentialID), req, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) DeleteSsoCredential(userID, credentialID string) error {
	return c.Delete(fmt.Sprintf("/users/%s/credentials/sso/%s", userID, credentialID))
}
