# Contributing to cmdctx

## Development setup

```bash
git clone https://github.com/slim/cmdctx
cd cmdctx
go mod download
make build
```

## Running tests

```bash
make test          # all tests with race detector
make test-short    # skip integration tests
make cover         # with HTML coverage report
```

## Code style

- `gofmt` formatting enforced (run `make fmt`)
- `go vet` must pass (run `make vet`)
- New packages must have package-level doc comments
- Exported functions must have doc comments

## Pull request guidelines

1. **Safety first** — no PR that weakens the safety model will be merged without extensive review
2. **Tests required** — new features must have corresponding tests
3. **No silent assumptions** — if you make an assumption, document it in `Assumptions` or code comments
4. **No freeform shell** — command generation must always go through the structured builder + policy chain
5. **Keep the policy immutable** — the blocked commands list in `policy.go` is the security baseline; user config can only extend it

## Architecture overview

See `docs/architecture.md` for the full design.

Key principle: AI is used for **intent parsing only**. The final command is always built by typed Go code, never by AI string generation.

## Adding a new command builder

1. Add a new `IntentType` constant in `internal/intent/intent.go`
2. Add a case to the `Build` switch in `internal/commands/builder.go`
3. Implement the builder function in a new file under `internal/commands/`
4. Add the new intent to the AI system prompt in `internal/intent/intent.go`
5. Write tests in `internal/commands/commands_test.go`

## Adding a new AI provider

1. Add a new config type in `internal/ai/`
2. Implement the `Provider` interface
3. Add the provider type to the factory in `internal/ai/provider.go`
4. Document it in `docs/provider-setup.md`
