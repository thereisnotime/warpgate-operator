package warpgate

import "fmt"

type TargetGroup struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

type TargetGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

func (c *Client) CreateTargetGroup(req TargetGroupRequest) (*TargetGroup, error) {
	var tg TargetGroup
	if err := c.Post("/target-groups", req, &tg); err != nil {
		return nil, err
	}
	return &tg, nil
}

func (c *Client) GetTargetGroup(id string) (*TargetGroup, error) {
	var tg TargetGroup
	if err := c.Get(fmt.Sprintf("/target-groups/%s", id), &tg); err != nil {
		return nil, err
	}
	return &tg, nil
}

func (c *Client) UpdateTargetGroup(id string, req TargetGroupRequest) (*TargetGroup, error) {
	var tg TargetGroup
	if err := c.Put(fmt.Sprintf("/target-groups/%s", id), req, &tg); err != nil {
		return nil, err
	}
	return &tg, nil
}

func (c *Client) DeleteTargetGroup(id string) error {
	return c.Delete(fmt.Sprintf("/target-groups/%s", id))
}
