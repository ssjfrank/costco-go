# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-09-23

The library and CLI are now focused on one job: downloading a complete Costco
purchase history to local JSON files. Signing in is the only manual step left.

### Added
- **Sign in with one console command**: Sign in to costco.com in your own browser, open Orders & Returns, and paste the command from `costco-cli -cmd snippet` into the DevTools console. It finds the sign-in Costco's order history app (MSAL.js) keeps in the page's storage and asks `signin.costco.com` for fresh tokens the way the app does. It then copies them to the clipboard as a block of short lines. `costco-cli` picks the block up from the clipboard by itself (`pbpaste`, PowerShell `Get-Clipboard`, `wl-paste`, `xclip`, `xsel`), or you can paste it. Short lines matter because macOS terminals cut a pasted line off at 1024 characters. This replaces the manual DevTools Network-tab copy. When Costco rejects the saved sign-in partway through a download, the steps are shown again and the download resumes afterwards.
- **`ConsoleSnippet()`** and **`ParseSignIn(text)`**: The library side of the above. `ParseSignIn` reads the block (tolerating console prefixes, indentation and CRLF) or a raw token response, and returns the `TokenResponse` that `ImportTokenResponse` accepts.
- **Optional sign-in through a window the tool opens** (`-browser-login`): Starts Chrome, Edge, Chromium or Brave with a throwaway profile on Costco's order history, which redirects to the sign-in page, and captures the OAuth token response the site receives over the DevTools protocol. Library: **`LoginWithBrowser(ctx, BrowserLoginOptions)`** and **`SignInStartURL`**.
- **`ErrNotAuthenticated`**: Wrapped by every error that only signing in again can fix (no tokens saved, a refused refresh token, a 401/403 from the API), so callers can tell it apart from an outage.
- **Prebuilt binaries**: Pushing a `v*` tag runs `.github/workflows/release.yml`, which builds macOS (Apple Silicon and Intel), Linux and Windows binaries with `scripts/build-release.sh` and publishes them with `SHA256SUMS` as a GitHub Release (suffixed tags become pre-releases). On a Mac, one `curl` command downloads the right build; no Go toolchain is needed. The darwin/arm64 binary carries the ad-hoc signature Go's linker generates, which Apple Silicon requires.
- **`costco-cli -version`**: Prints the version and the commit a release build came from.
- **Container image** (`Dockerfile`): Runs a download with only the token file mounted. It is a distroless static image of about 16 MB that runs as non-root with `-non-interactive`, reads the token file from `/secrets/tokens.json`, and writes to `/data/costco-history`. Build it straight from the Git URL; `.dockerignore` keeps `costco-history/`, `token.json` and `.costco/` out of the build context.
- **`COSTCO_TOKEN_FILE`** (`TokenFileEnv`, `TokenFilePath()`): Points loading and saving at any token file instead of `~/.costco/tokens.json`, without touching `~/.costco` at all. Tokens are rewritten in place, which a single-file bind mount requires; a test holds that. If Docker has created a directory where the file should be (`-v` with a missing source does that), the error explains what happened. Sign-in and not-signed-in messages name the token file.
- **`costco-cli -cmd login`**, **`-cmd snippet`**, **`-browser`** and **`-non-interactive`**: Sign in without downloading; print the console command; choose the browser for `-browser-login`; never ask for a sign-in (for unattended runs, which then fail with instructions instead).
- **`Client.DownloadHistory(ctx, HistoryOptions)`**: Downloads every online order and warehouse receipt in a date range. The range is walked newest-first in windows small enough for Costco's API to accept, online orders are paged until exhausted, and each receipt is expanded into its full line-item detail. A failure in the newest window aborts the run (that almost always means expired tokens); later failures are collected in `History.Warnings` so one unavailable record cannot discard years of downloaded data.
- **`Client.FetchAllOnlineOrders(ctx, startDate, endDate, pageSize)`**: Pages through every online order in a date range.
- **`FileStore`**: Writes history to a directory as `orders/<order number>.json`, `receipts/<barcode>.json`, combined `orders.json` / `receipts.json`, and a `manifest.json` index. Records are written as they arrive, so an interrupted download resumes instead of starting over. Files and directories are created user-only (0600/0700) because receipts contain membership numbers and payment descriptions.
- **`SplitDateRange(since, until, windowDays)`**: Splits a date range into contiguous, non-overlapping windows, newest first.
- **`History`, `HistoryOptions`, `HistoryStore`, `Manifest`, `DateWindow`**: Supporting types for the download flow.
- **`ErrNoOrderData`**: Sentinel error letting callers distinguish "this window has no orders" from a real failure.
- **`costco-cli` download command** (now the default): `costco-cli` with no arguments downloads everything into `./costco-history`. Flags: `-out`, `-since`, `-until`, `-window`, `-page-size`, `-delay`, `-retries`, `-no-items`, `-force`, `-quiet`. Exits non-zero when any record could not be downloaded.

### Changed
- **BREAKING — CLI commands**: `-cmd orders`, `-cmd receipts` and `-cmd receipt-detail` are gone. The downloaded JSON contains the same data, and `-cmd` now defaults to `download`. `setup`, `import-token` and `info` are unchanged.
- **`GetReceipts` date handling**: Dates given as `YYYY-MM-DD` are converted to the `M/DD/YYYY` format Costco's receipt API requires, so both formats now work. Previously an ISO date was forwarded unchanged and silently returned nothing.
- **`CostcoClient` interface**: Now describes the download methods alongside the three API primitives.
- **Sign-in failures stop a download**: `DownloadHistory` treats `ErrNotAuthenticated` like cancellation — it is never retried and never recorded as a warning, because every later request would fail the same way.
- **`costco-cli -cmd setup` is optional**: The warehouse number defaults to `847` and the email is only used for logging.
- **`costco-cli -cmd import-token`** accepts the console command's block as well as a raw token response, so `import-token < token.json` works with either.

### Security
- The tool never sees your password, passkey or security key, only OAuth tokens Costco issued. The console command's only network request goes to `signin.costco.com`, the same refresh the order history app makes, and it copies nothing when Costco refuses the sign-in. The docs warn that pasting untrusted console commands is a common account-theft trick.
- The `-browser-login` window uses a fresh, temporary browser profile that is deleted as soon as the tokens are captured, so no Costco session cookies are left behind. It accepts a token response only from the token endpoint on `signin.costco.com`.
- The `-browser-login` window is started with `--disable-blink-features=AutomationControlled`. Chrome otherwise sets `navigator.webdriver` for any window that has a DevTools connection, and sign-in pages may refuse such windows even though a person is the one signing in.
- New dependency: `github.com/coder/websocket` (ISC licence, no dependencies of its own), used only to talk to the local browser over its DevTools socket on `127.0.0.1`.

### Removed
- **BREAKING — spending analytics helpers**: `GetAllTransactionItems`, `GetItemHistory`, `GetSpendingSummary` and `GetFrequentItems`, along with the `TransactionWithItems`, `ItemPurchase`, `SpendingByDepartment` and `FrequentItem` types. Each re-downloaded the whole history to compute one aggregate; the same analysis now runs offline over the downloaded JSON.

### Fixed
- **CI**: The workflows pinned Go 1.21/1.22 while `go.mod` requires 1.24, so every job would have failed; they now read the version from `go.mod`. The build job also listed macOS and Windows in its matrix but always ran on Ubuntu; it now runs on each OS and checks the binary with `-version`.
- **Client tests**: Several tests still built clients without tokens after password-grant authentication was removed in 0.3.11, so they failed before reaching their test server.

[1.0.0]: https://github.com/eshaffer321/costco-go/compare/v0.3.11...v1.0.0

## [0.3.11] - 2026-06-20

### Fixed
- **OAuth B2C Policy Update**: Updated token endpoint from B2C policy `201` to `209` to match Costco's current authentication setup.
- **Token Refresh**: Removed unsupported `scope` parameter from refresh token requests, which was causing `AADB2C90117` errors. The `refresh_token` grant type should only include `grant_type`, `client_id`, and `refresh_token`.
- **Password Grant Removal**: Removed password-based authentication entirely. OAuth2 password grant no longer works with Costco's current setup. Users must use token import from browser: `costco-cli -cmd import-token`.

[0.3.11]: https://github.com/eshaffer321/costco-go/compare/v0.3.10...v0.3.11

## [0.3.10] - 2026-05-27

### Fixed
- **`NetDiscounts` word-overlap fallback**: Added a fourth matching strategy for cases where a coupon references a parent by a generic category name that shares no substring with the branded product description. For example, a coupon with `/AAA BATTERY` now correctly matches an item described as `DURACELL AAA` via the shared token `AAA`. Words shorter than 3 characters are excluded to avoid false positives. When multiple candidates share words with the coupon reference, the item with the most words in common is chosen.

[0.3.10]: https://github.com/eshaffer321/costco-go/compare/v0.3.9...v0.3.10

## [0.3.9] - 2026-05-26

### Added
- **`NetDiscounts(items []ReceiptItem) ([]ReceiptItem, []ReceiptItem)`**: New exported utility function that applies Costco discount line items to their parent items using a three-level matching strategy: (1) exact item-number match, (2) exact description match (case-insensitive), (3) substring/contains match. Returns the netted regular items and any orphaned discounts that could not be matched. Fixes the case where a coupon references a parent by a partial description token (e.g. `/AAA BATTERY`) that does not exactly match the parent's full description (e.g. `AA/AAA BATTERY`).

[0.3.9]: https://github.com/eshaffer321/costco-go/compare/v0.3.8...v0.3.9

## [0.3.8] - 2026-04-23

### Added
- **`costco-cli -cmd import-token`**: New CLI command to bootstrap authentication by pasting the JSON response from the Costco token endpoint. Eliminates the need to manually edit `~/.costco/tokens.json`. See README for instructions.
- **`ImportTokenResponse()`**: New exported library function that converts a `TokenResponse` into `StoredTokens`, parsing the JWT expiry and calculating refresh token expiry from `refresh_token_expires_in`.

[0.3.8]: https://github.com/eshaffer321/costco-go/compare/v0.3.7...v0.3.8

## [0.3.7] - 2026-04-23

### Fixed
- **Version Drift**: Sync `constants.go` and `README.md` badge to `0.3.7` — both were missed during the `v0.3.6` re-tag and still reported `0.3.5`.
- **go.mod**: Promoted `golang.org/x/term` from indirect to direct dependency.

### Added
- **CI Version Gate**: New `version-consistency` CI job that fails if `constants.go`, `CHANGELOG.md`, and the `README.md` badge don't all report the same version, preventing this class of drift in the future.

[0.3.7]: https://github.com/eshaffer321/costco-go/compare/v0.3.6...v0.3.7

## [0.3.6] - 2026-04-23

### Fixed
- **Module Checksum**: Re-tagged to resolve a `go.sum` checksum mismatch caused by a force-push to the `v0.3.5` tag. No code changes from v0.3.5.

[0.3.6]: https://github.com/eshaffer321/costco-go/compare/v0.3.5...v0.3.6

## [0.3.5] - 2026-03-16

### Fixed
- **Token Refresh Authentication**: Updated Azure AD B2C policy version from `184` to `201` to match Costco's current authentication policy. This fixes token refresh failures that occurred after Costco updated their authentication infrastructure.

[0.3.5]: https://github.com/eshaffer321/costco-go/compare/v0.3.3...v0.3.5

## [0.3.3] - 2025-12-07

### Fixed
- **Token Refresh Authentication**: Updated Azure AD B2C policy version from `157` to `184` to match Costco's current authentication policy. This fixes token refresh failures that occurred after Costco updated their authentication infrastructure.

[0.3.3]: https://github.com/eshaffer321/costco-go/compare/v0.3.2...v0.3.3

## [0.3.2] - 2025-11-17

### Added
- **Comprehensive Test Coverage**: Significantly increased test coverage from 62.8% to 82.8%
  - Added integration tests for analytics functions (`GetItemHistory`, `GetSpendingSummary`, `GetFrequentItems`) - now calling actual client methods instead of testing logic inline
  - Added comprehensive tests for config persistence functions (`SaveConfig`, `LoadConfig`, `SaveTokens`, `LoadTokens`, `ClearTokens`, `GetConfigInfo`)
  - New test file `config_persistence_test.go` with 11 test cases covering config file operations, token management, and edge cases
  - Tests now cover file permissions, JSON validation, non-existent files, and expired tokens

### Changed
- **Improved Test Quality**: Rewrote existing analytics tests to call actual client methods, ensuring real-world usage patterns are tested

[0.3.2]: https://github.com/eshaffer321/costco-go/compare/v0.3.1...v0.3.2

## [0.3.1] - 2025-10-24

### Fixed
- **GetParentItemNumber() Whitespace Handling**: Fixed bug where `GetParentItemNumber()` returned item numbers with leading/trailing whitespace when Costco's API includes spaces after the slash (e.g., "/ 1857091"). The method now trims whitespace, making item numbers usable for direct lookups without additional processing by consumers.

### Added
- Test coverage for discount items with whitespace variations in descriptions

[0.3.1]: https://github.com/eshaffer321/costco-go/compare/v0.3.0...v0.3.1

## [0.3.0] - 2025-10-23

### Added
- **Discount Line Item Helpers**: Added helper methods to identify and process discount line items in receipts
  - `ReceiptItem.IsDiscount()` - Returns true if the item is a discount/adjustment line item
  - `ReceiptItem.GetParentItemNumber()` - Returns the item number the discount applies to
- **Comprehensive Documentation**: Added "Handling Discount Line Items" section to README with:
  - Explanation of discount item characteristics
  - Distinction between discounts and returns
  - Real-world examples and use cases
  - Code examples for calculating net item amounts
- **Test Coverage**: Added comprehensive tests for discount helpers including:
  - Edge case testing (returns, empty descriptions, slash in middle of string)
  - Real-world data from actual Costco receipts
  - Workflow tests demonstrating practical usage patterns

### Fixed
- Clarified that discount amounts are already factored into receipt subtotals to prevent double-counting in consuming applications

[0.3.0]: https://github.com/eshaffer321/costco-go/compare/v0.2.0...v0.3.0

## [0.2.0] - 2025-10-20

### Added
- **New Named Types**: Replaced anonymous struct return types with proper named types for better usability:
  - `ItemPurchase` - for `GetItemHistory()` results
  - `SpendingByDepartment` - for `GetSpendingSummary()` results
  - `FrequentItem` - for `GetFrequentItems()` results
- **Client Interface**: Added `CostcoClient` interface for better testability and mocking support
- **Comprehensive Godoc**: Added detailed documentation to all exported types and functions with examples
- **Domain-Specific Files**: Organized types into logical domain files:
  - `orders.go` - Order-related types
  - `receipts.go` - Receipt-related types
  - `analytics.go` - Analytics types
  - `options.go` - Configuration types
  - `graphql.go` - GraphQL types
  - `interface.go` - Client interface definition
- **Migration Guide**: Added `MIGRATION_v0.2.0.md` with comprehensive upgrade instructions for AI agents
- **Audit Documentation**: Added `AUDIT.md` with detailed codebase analysis and recommendations

### Changed
- **Breaking**: `GetItemHistory()` now returns `[]ItemPurchase` instead of anonymous struct
- **Breaking**: `GetSpendingSummary()` now returns `map[int]SpendingByDepartment` instead of anonymous struct
- **Breaking**: `GetFrequentItems()` now returns `[]FrequentItem` instead of anonymous struct
- **File Organization**: Split `types.go` into domain-specific files for better code organization
- **Performance**: Replaced O(n²) bubble sort with O(n log n) stdlib `sort.Slice` in `GetFrequentItems()`
- **Code Quality**: Removed duplicate type definitions between files

### Removed
- **Breaking**: Removed `types.go` (types moved to domain-specific files)
- Removed duplicate `TransactionWithItems` definition from `helpers.go` (now only in `analytics.go`)

### Improved
- Better code organization with clear separation of concerns
- Enhanced discoverability with logical type grouping
- Improved testability with interface-based design
- Better IDE autocomplete and documentation support
- Cleaner, more maintainable codebase structure

### Migration Notes
See `MIGRATION_v0.2.0.md` for detailed upgrade instructions. Key changes:
- Update code using `GetItemHistory()`, `GetSpendingSummary()`, or `GetFrequentItems()` to use new named types
- No changes needed for core API methods (`GetOnlineOrders`, `GetReceipts`, `GetReceiptDetail`)
- Import path remains the same: `github.com/costco-go/pkg/costco`

[0.2.0]: https://github.com/eshaffer321/costco-go/compare/v0.1.1...v0.2.0

## [0.1.1] - 2025-10-19

### Fixed
- Improved receipt fetching logging to eliminate misleading ERROR logs during normal operation
- Optimized `GetReceipts()` fallback logic to try object format first (the format Costco API currently returns), reducing unnecessary failed attempts
- Added clear diagnostic logging with emojis to identify if array fallback is ever needed
- Reduced log noise by changing generic GraphQL decode errors to DEBUG level

[0.1.1]: https://github.com/eshaffer321/costco-go/compare/v0.1.0...v0.1.1

## [0.1.0] - 2025-10-19

### Added
- Initial release of Costco Go client library
- Authentication using Azure AD B2C OAuth2/OIDC flow
- Order history retrieval via GraphQL API
- Support for refresh tokens
- Structured logging with slog (silent by default using io.Discard)
- CLI tool for command-line usage
- Configurable warehouse selection
- Pagination support for order history

### Fixed
- Azure AD B2C authentication flow
- GraphQL array response handling
- Linting and formatting issues
- Test failures with structured logging

[0.1.0]: https://github.com/costco-go/compare/v0.1.0
