// Package validate contains input validation helpers for HTTP API.
package validate

import (
	"fmt"
	"net/url"
	"strings"
)

const maxTicketSaleURLLen = 1024

// TicketSaleURL validates an absolute http(s) URL for teams.ticket_sale_url.
func TicketSaleURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("ticket_sale_url must not be empty")
	}
	if len(s) > maxTicketSaleURLLen {
		return "", fmt.Errorf("ticket_sale_url exceeds maximum length")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("ticket_sale_url is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("ticket_sale_url must use http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("ticket_sale_url must include a host")
	}
	return s, nil
}
