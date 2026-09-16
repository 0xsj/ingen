package custody

// handlingEventCoordinator serializes handling-event read-modify-publish
// workflows for one custody record. It is intentionally internal so custom
// event stores are not silently treated as coordinated implementations.
type handlingEventCoordinator interface {
	withHandlingEventLock(custodyID string, fn func() error) error
}

// handlingEventAppender publishes one event while the caller already holds
// the per-custody handling-event lock.
type handlingEventAppender interface {
	appendHandlingEvent(event HandlingEvent) error
}

func withHandlingEventLock(events HandlingEventStore, custodyID string, fn func() error) error {
	if coordinator, ok := events.(handlingEventCoordinator); ok {
		return coordinator.withHandlingEventLock(custodyID, fn)
	}
	return fn()
}

func appendHandlingEventWhileLocked(events HandlingEventStore, event HandlingEvent) error {
	if appender, ok := events.(handlingEventAppender); ok {
		return appender.appendHandlingEvent(event)
	}
	return events.AppendEvent(event)
}
