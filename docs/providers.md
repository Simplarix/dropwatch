# Extending Availability Providers

Registrar adapters implement the interface in `internal/availability`:

```go
type AvailabilityProvider interface {
    Name() string
    Check(ctx context.Context, domain string) (AvailabilityResult, error)
}
```

The built-in RDAP provider is the primary source. `MockProvider` enables deterministic tests without network access.

A new adapter should:

- send only normalized ASCII/Punycode domains to its API;
- bind every request to the supplied context;
- limit timeouts, redirects, and response sizes;
- require HTTPS, reject scheme downgrades, resolve targets before dialing, and block loopback, private, link-local, metadata, and other non-public addresses;
- represent rate limits, including `Retry-After`;
- avoid logging tokens, credential-bearing URLs, or unfiltered response bodies;
- set `Registered` whenever a credible registration signal exists;
- set `Available` only for an explicit, authoritative positive result;
- return ambiguous responses as errors or unknown results.

The application uses the conservative rule when providers disagree: a registration signal always wins over an availability signal. Even agreement among multiple providers is not a registration guarantee.

To enable a provider, construct it in `internal/app` and pass it as an additional provider to `NewChecker`. Add its configuration to `internal/config`; keep secret example values empty.

Provider tests must use a mock or local test server and cover timeout, rate-limit, malformed-response, registered, and available outcomes.
