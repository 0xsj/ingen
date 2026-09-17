# Sanitized live Herdr fixture

This fixture preserves the shape of a real Herdr 0.9.0
`pane.agent_status_changed` callback observed by the compatibility probe. Host
and filesystem identifiers were replaced with fixture values; the fixture is
evidence of envelope shape, not a normative host contract.

Validate it with:

```sh
sh ../../verify-captures.sh .
```
