package governance

import (
	"fmt"
	"strings"
)

// AuthorityGrant is an effective-dated actor-to-role membership assertion
// supplied by a caller-owned authority source. ValidUntil is exclusive; an
// empty value means the grant has no recorded expiry.
type AuthorityGrant struct {
	Actor      string `json:"actor"`
	Role       string `json:"role"`
	ValidFrom  string `json:"valid_from"`
	ValidUntil string `json:"valid_until,omitempty"`
}

// TimeScopedAuthority adapts effective-dated membership data to
// AuthorityVerifier. It is useful for testing or for a provider adapter after
// the provider has authenticated and normalized its response.
type TimeScopedAuthority struct {
	Grants []AuthorityGrant
}

func (authority TimeScopedAuthority) Validate() error {
	for index, grant := range authority.Grants {
		path := fmt.Sprintf("grants[%d]", index)
		if strings.TrimSpace(grant.Actor) == "" {
			return fmt.Errorf("%s.actor is required", path)
		}
		if strings.TrimSpace(grant.Role) == "" {
			return fmt.Errorf("%s.role is required", path)
		}
		validFrom, err := parseUTC(grant.ValidFrom)
		if err != nil {
			return fmt.Errorf("%s.valid_from must be an RFC3339 UTC timestamp: %w", path, err)
		}
		if grant.ValidUntil == "" {
			continue
		}
		validUntil, err := parseUTC(grant.ValidUntil)
		if err != nil {
			return fmt.Errorf("%s.valid_until must be an RFC3339 UTC timestamp: %w", path, err)
		}
		if !validUntil.After(validFrom) {
			return fmt.Errorf("%s.valid_until must be after valid_from", path)
		}
	}
	return nil
}

// Verify implements AuthorityVerifier using inclusive valid_from and
// exclusive valid_until bounds.
func (authority TimeScopedAuthority) Verify(actor, role, at string) (bool, error) {
	decisionAt, err := parseUTC(at)
	if err != nil {
		return false, fmt.Errorf("authority decision timestamp must be RFC3339 UTC: %w", err)
	}
	if err := authority.Validate(); err != nil {
		return false, err
	}
	actor = strings.TrimSpace(actor)
	role = strings.TrimSpace(role)
	for _, grant := range authority.Grants {
		if strings.TrimSpace(grant.Actor) != actor || strings.TrimSpace(grant.Role) != role {
			continue
		}
		validFrom, _ := parseUTC(grant.ValidFrom)
		if decisionAt.Before(validFrom) {
			continue
		}
		if grant.ValidUntil != "" {
			validUntil, _ := parseUTC(grant.ValidUntil)
			if !decisionAt.Before(validUntil) {
				continue
			}
		}
		return true, nil
	}
	return false, nil
}
