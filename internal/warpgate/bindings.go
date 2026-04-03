package warpgate

import "fmt"

// UserRole binding operations.

func (c *Client) CreateUserRole(userID, roleID string) error {
	resp, err := c.doRequest("POST", fmt.Sprintf("/users/%s/roles/%s", userID, roleID), nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{StatusCode: resp.StatusCode, Body: "failed to create user-role binding"}
	}
	return nil
}

func (c *Client) DeleteUserRole(userID, roleID string) error {
	return c.Delete(fmt.Sprintf("/users/%s/roles/%s", userID, roleID))
}

func (c *Client) ListUserRoles(userID string) ([]Role, error) {
	var roles []Role
	if err := c.Get(fmt.Sprintf("/users/%s/roles", userID), &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// TargetRole binding operations.

func (c *Client) CreateTargetRole(targetID, roleID string) error {
	resp, err := c.doRequest("POST", fmt.Sprintf("/targets/%s/roles/%s", targetID, roleID), nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{StatusCode: resp.StatusCode, Body: "failed to create target-role binding"}
	}
	return nil
}

func (c *Client) DeleteTargetRole(targetID, roleID string) error {
	return c.Delete(fmt.Sprintf("/targets/%s/roles/%s", targetID, roleID))
}

func (c *Client) ListTargetRoles(targetID string) ([]Role, error) {
	var roles []Role
	if err := c.Get(fmt.Sprintf("/targets/%s/roles", targetID), &roles); err != nil {
		return nil, err
	}
	return roles, nil
}
