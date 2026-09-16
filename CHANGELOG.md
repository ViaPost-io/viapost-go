# Changelog

All notable changes to this project are documented in this file. This project follows
[Semantic Versioning](https://semver.org/).

## [0.3.0] - 2026-09-16

### Breaking changes

- `api.GetSegmentsIDContacts` now returns the documented paginated `*api.ContactList` on HTTP
  `200` instead of the incorrect generated `*api.GetSegmentsIDContactsNoContent` on HTTP `204`.

### Changed

- Synchronize the generated client with the public OpenAPI contract reviewed at
  `ViaPost-io/base-code@891adebbe79a26178fb780ec986172c890a5e261`.
- Add a regression test that decodes the documented segment-contact list response.
- Keep JSON responses capped at 8 MiB while allowing raw RFC822 messages and CSV exports up to
  the API-supported 40 MiB by default, with a separately configurable bounded limit.
- Redact the active API key from error bodies, parsed messages, request IDs, and response headers.

## [0.2.0] - 2026-09-16

### Breaking changes

- The generated low-level `api` package now types webhook event collections as
  `[]WebhookSubscribableEventType` instead of `[]string`.
- The generated `api.Invoker` interface includes the newly published public operations.
  Implementations of this low-level interface must add those methods.
- `Webhooks.Create` now rejects non-HTTPS, credentialed, fragmented, or hostless webhook
  destinations locally, matching the public API contract.

### Changed

- Synchronize the generated client with the public OpenAPI contract reviewed at
  `ViaPost-io/base-code@5eed29795633d3c4509851ce8638ef97d2370b07`.
- Expose the latest public suppressions, webhook operations, status subscription,
  raw-content, and message-detail operations through the low-level client.

## [0.1.0] - 2026-09-11

### Added

- Official server-side Go client with Bearer API key authentication.
- Ergonomic services for email, messages, domains, templates, webhooks, automations, and usage.
- Typed API errors with HTTP status, error body, headers, and request ID.
- Context cancellation, configurable timeout, base URL, HTTP client, and User-Agent.
- Locally reproducible ogen-generated OpenAPI client and drift check.
