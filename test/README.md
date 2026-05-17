# Test secrets

All fabricated credentials for the test suite live in **`test/.env`** (gitignored).
Committed fixtures use `{{KEY}}` placeholders that are expanded at runtime from that file.

## Setup

```bash
cp test/.env.example test/.env
# Edit test/.env — paste doc-shaped sample keys (Stripe/AWS/jwt.io public examples).
make test
```

Or run `make test-setup` to copy the example file if `.env` is missing.

## CI

Create `test/.env` before `go test`, for example from a GitHub Actions secret:

```yaml
- run: printf '%s' "${{ secrets.LOCKIE_TEST_ENV }}" > test/.env
- run: make test
```

Set `LOCKIE_TEST_ENV` to an alternate path if needed.
