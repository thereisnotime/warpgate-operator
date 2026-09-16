package warpgate

import "fmt"

type Ticket struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Description string `json:"description,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
	Target      string `json:"target,omitempty"`
	UsesLeft    *int   `json:"uses_left,omitempty"`
	SelfService bool   `json:"self_service,omitempty"`
	Expiry      string `json:"expiry,omitempty"`
	Created     string `json:"created,omitempty"`
}

type TicketAndSecret struct {
	Ticket Ticket `json:"ticket"`
	Secret string `json:"secret"`
}

type TicketCreateRequest struct {
	Username     string `json:"username,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	TargetName   string `json:"target_name,omitempty"`
	TargetID     string `json:"target_id,omitempty"`
	Expiry       string `json:"expiry,omitempty"`
	NumberOfUses *int   `json:"number_of_uses,omitempty"`
	Description  string `json:"description,omitempty"`
}

func (c *Client) CreateTicket(req TicketCreateRequest) (*TicketAndSecret, error) {
	var result TicketAndSecret
	if err := c.Post("/tickets", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteTicket(id string) error {
	return c.Delete(fmt.Sprintf("/tickets/%s", id))
}
