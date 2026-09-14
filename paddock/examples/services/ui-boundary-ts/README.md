# UI boundary fixture

This deliberately small TypeScript fixture exercises the same boundary used by
the Overwatch UI proposal. The good subject keeps service code pointed at
transport and kernel types; the violating subject imports a presentation
component from a service module.
