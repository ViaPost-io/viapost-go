# Changelog

All notable changes to this project are documented in this file. This project follows
[Semantic Versioning](https://semver.org/).

## [0.5.0] - Unreleased

### Fixed

- Normalize only the `SendCustomEventRequest` identifier-selection `anyOf` in the temporary
  ogen input. This keeps the versioned OpenAPI snapshot and public wire contract exact while
  allowing the pinned ogen `v1.24.0` generator to produce both `contact_id` and `email` fields
  when the reviewed public contract is synchronized.

## [0.4.0] - 2026-09-21

### Breaking changes

- The generated low-level `api` package now includes the current public contacts import,
  tracking-domain, inbound, audience-segment, and broadcast foundations. Implementations of
  low-level generated interfaces must add the corresponding operations.
- `api.DynamicSegmentMembershipErrorError.Code` is now the documented `string` constant instead
  of `jx.Raw`; consumers assigning or comparing raw JSON must migrate to a string.

### Changed

- Synchronize the generated client with the published public OpenAPI contract at
  `ViaPost-io/base-code@a8d78b4dc2140d3ceee2dbc52eb9ee438b640560`.
- Keep the v0.3 segment-union normalization and validation intact while adding a temporary,
  explicit type for otherwise bare OpenAPI string constants that ogen v1.24 cannot encode.

## [0.3.0] - 2026-09-16

### Breaking changes

- `api.GetSegmentsIDContacts` now returns the documented paginated `*api.ContactList` on HTTP
  `200` instead of the incorrect generated `*api.GetSegmentsIDContactsNoContent` on HTTP `204`.
- The low-level generated segment unions (`api.CreateSegmentRequest`, `api.Segment`, and
  `api.SegmentDefinition`) are now lossless `map[string]jx.Raw` values because ogen cannot
  faithfully model the documented recursive object `oneOf` schemas. Migrate creation calls from
  generated struct fields to JSON raw fields, for example:

  ```go
  request := api.CreateSegmentRequest{
      "name":       jx.Raw(`"VIP customers"`),
      "kind":       jx.Raw(`"dynamic"`),
      "definition": jx.Raw(`{"all":[{"field":"email","operator":"contains","value":"@example.com"}]}`),
  }
  ```

  The SDK validates the documented static/dynamic variants, all seven predicate forms, group
  depth and the 100-predicate bound before sending; inspect raw response fields with `encoding/json`
  or the generated `Validate` helpers.

### Changed

- Synchronize the generated client with the public OpenAPI contract reviewed at
  `ViaPost-io/base-code@207702c8309db84354ae6a5f7a9f3042e21050a0`.
- Include the published contacts import, domain health and inbound configuration,
  message events and cancellation, batch send, and segment preview operations.
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
