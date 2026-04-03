package warpgate

import "fmt"

type CredentialPolicy struct {
	HTTP     []string `json:"http,omitempty"`
	SSH      []string `json:"ssh,omitempty"`
	MySQL    []string `json:"mysql,omitempty"`
	Postgres []string `json:"postgres,omitempty"`
}

type User struct {
	ID               string            `json:"id,omitempty"`
	Username         string            `json:"username"`
	Description      string            `json:"description,omitempty"`
	CredentialPolicy *CredentialPolicy `json:"credential_policy,omitempty"`
}

type UserCreateRequest struct {
	Username    string `json:"username"`
	Description string `json:"description,omitempty"`
}

type UserUpdateRequest struct {
	Username         string            `json:"username"`
	Description      string            `json:"description,omitempty"`
	CredentialPolicy *CredentialPolicy `json:"credential_policy,omitempty"`
}

func (c *Client) CreateUser(req UserCreateRequest) (*User, error) {
	var user User
	if err := c.Post("/users", req, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) GetUser(id string) (*User, error) {
	var user User
	if err := c.Get(fmt.Sprintf("/users/%s", id), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) GetUserByUsername(username string) (*User, error) {
	users, err := c.ListUsers(username)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Username == username {
			return &u, nil
		}
	}
	return nil, &APIError{StatusCode: 404, Body: fmt.Sprintf("user %q not found", username)}
}

func (c *Client) UpdateUser(id string, req UserUpdateRequest) (*User, error) {
	var user User
	if err := c.Put(fmt.Sprintf("/users/%s", id), req, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) DeleteUser(id string) error {
	return c.Delete(fmt.Sprintf("/users/%s", id))
}

func (c *Client) ListUsers(search string) ([]User, error) {
	path := "/users"
	if search != "" {
		path += "?search=" + search
	}
	var users []User
	if err := c.Get(path, &users); err != nil {
		return nil, err
	}
	return users, nil
}
