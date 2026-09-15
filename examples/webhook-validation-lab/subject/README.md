# Webhook validation subject

The subject exposes only the public HTTP boundary used by the lab contract.
Its in-memory duplicate set makes idempotency visible across requests while
keeping the implementation small enough for black-box verification.
