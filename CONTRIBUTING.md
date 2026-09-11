# Contributing

Thanks for helping make agent cost claims easier to verify.

## Local checks

```sh
gofmt -w cmd internal
go test -race ./...
go vet ./...
```

Keep runtime dependencies out of the CLI unless a concrete need justifies one. New provider adapters should come with small, synthetic usage fixtures that show the provider's accounting semantics.

## Good contributions

- Tests for ambiguous or changing usage schemas.
- Native session importers that distinguish per-call usage from cumulative totals.
- Better CI reports without hiding failed-task spend.
- Reproducible examples of an optimization increasing total task cost.

Open an issue before implementing a new accounting category or changing the gate policy. These affect existing CI users.

## Reporting bugs

Include the CLI version, operating system, expected result, and a minimal synthetic trace plus price book. Remove prompts, private identifiers, credentials, and customer data. Do not attach your complete agent session.

## Conduct

Be respectful and specific. Critique code and claims, not people. Harassment and discriminatory behavior are not welcome.
