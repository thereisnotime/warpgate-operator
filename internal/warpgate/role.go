package warpgate

import "fmt"

type Role struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type RoleCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (c *Client) CreateRole(req RoleCreateRequest) (*Role, error) {
	var role Role
	if err := c.Post("/roles", req, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (c *Client) GetRole(id string) (*Role, error) {
	var role Role
	if err := c.Get(fmt.Sprintf("/role/%s", id), &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (c *Client) GetRoleByName(name string) (*Role, error) {
	roles, err := c.ListRoles(name)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if r.Name == name {
			return &r, nil
		}
	}
	return nil, &APIError{StatusCode: 404, Body: fmt.Sprintf("role %q not found", name)}
}

func (c *Client) UpdateRole(id string, req RoleCreateRequest) (*Role, error) {
	var role Role
	if err := c.Put(fmt.Sprintf("/role/%s", id), req, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (c *Client) DeleteRole(id string) error {
	return c.Delete(fmt.Sprintf("/role/%s", id))
}

func (c *Client) ListRoles(search string) ([]Role, error) {
	path := "/roles"
	if search != "" {
		path += "?search=" + search
	}
	var roles []Role
	if err := c.Get(path, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}
