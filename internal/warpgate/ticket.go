package warpgate

import "fmt"

type Ticket struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	Description string `json:"description,omitempty"`
	Target      string `json:"target,omitempty"`
	UsesLeft    string `json:"uses_left,omitempty"`
	Expiry      string `json:"expiry,omitempty"`
	Created     string `json:"created,omitempty"`
}

type TicketAndSecret struct {
	Ticket Ticket `json:"ticket"`
	Secret string `json:"secret"`
}

type TicketCreateRequest struct {
	Username     string `json:"username,omitempty"`
	TargetName   string `json:"target_name,omitempty"`
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
