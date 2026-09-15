package delivery

import "context"

// Publisher delivers one provider-neutral Nublar decision. Implementations
// own destination-specific retries and must not mutate the source decision.
type Publisher interface {
	Publish(ctx context.Context, decision Decision) error
}
