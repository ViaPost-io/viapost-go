# Contributing

Thank you for improving the ViaPost Go SDK.

## Requirements

- Go 1.25+
- Git

## Workflow

1. Create a focused branch from `main`.
2. Add a failing test that describes the behavior.
3. Implement the smallest compatible change and refactor with the suite green.
4. Run the complete verification:

```bash
make verify
git diff --check
```

When `openapi.yaml` changes, update it from the canonical ViaPost public contract, run
`make generate`, review generated changes, and update the checksum in `README.md`. Do not edit
files under `api/` manually. Generation uses the pinned ogen tool and the local snapshot; remote
schema loading is disabled.

Do not include API keys, customer data, production payloads, or other secrets in code, fixtures,
issues, or pull requests. Report vulnerabilities according to [SECURITY.md](./SECURITY.md).
