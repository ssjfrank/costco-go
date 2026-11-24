# Costco Go Client

[![Version](https://img.shields.io/badge/version-1.1.0-blue.svg)](https://github.com/eshaffer321/costco-go/releases/tag/v1.1.0)

Download your complete Costco purchase history — every online order and every
warehouse receipt, down to the individual line items — into local JSON files.

Costco's website only shows a few months at a time behind a date picker. This
tool walks the entire history for you and writes it to disk, so you can search,
analyse or archive years of purchases offline.

## Quick start

中文用户请看分步指南：[docs/GUIDE.zh-CN.md](docs/GUIDE.zh-CN.md)

1. Download the program. On a Mac, in Terminal (Apple Silicon and Intel alike):

```bash
mkdir -p ~/costco && cd ~/costco
ARCH=$(uname -m | sed 's/x86_64/amd64/')
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/ssjfrank/costco-go/releases/latest | sed -n 's#.*/tag/v##p')
curl -fL -o costco-cli "https://github.com/ssjfrank/costco-go/releases/download/v$VERSION/costco-cli-$VERSION-darwin-$ARCH"
chmod +x costco-cli
```

   `VERSION` is the latest release (pre-releases are skipped); every file on
   the [Releases](https://github.com/ssjfrank/costco-go/releases) page carries
   its version in its name, such as `costco-cli-1.1.0-windows-amd64.exe`.
   Linux and Windows builds are there too, or build from
   source with Go 1.24+: `go build -o costco-cli ./cmd/costco-cli`.
2. Run it:

```bash
./costco-cli
```

3. A browser window opens on Costco's sign-in page. Sign in the way you always
   do. The window closes by itself and the download starts:

```
You are not signed in to Costco yet, or your last sign-in has expired.
A browser window has opened on Costco's sign-in page. Sign in there with any method you normally use;
the window closes by itself once your order history starts loading.
If Costco still remembers you in that window, it closes without you doing anything.
Its cookies are kept on this computer only, in /Users/you/.costco/browser-profile;
delete that folder to remove them.
✓ Signed in. You won't need to sign in again until about 2026-12-22.

Downloading Costco history from 2016-09-24 to 2026-09-24 into costco-history

[1/11] 2025-08-07 to 2026-08-06: online orders
[1/11] 2025-08-07 to 2026-08-06: 34 online order(s)
[1/11] 2025-08-07 to 2026-08-06: warehouse receipts
[1/11] 2025-08-07 to 2026-08-06: 52 receipt(s)
...

Downloaded 214 online orders and 388 warehouse receipts (7431 receipt line items)
  Date range:          2016-08-06 to 2026-08-06
  Online order total:  $41203.87
  Receipt total:       $76914.02
  Saved to:            costco-history
  Start here:          costco-history/manifest.json
```

It reaches back ten years and writes everything it finds into
`./costco-history`. Later runs reuse the sign-in for about 90 days and open no
window at all. When it runs out, the window opens again.

The window needs Chrome, Edge, Chromium or Brave. Without one of them, on a
machine with no display, or if the window does not work, `costco-cli` shows a
command to run in your own browser instead; see [Signing in](#signing-in).

## What you get

```
costco-history/
├── manifest.json          Summary and index of everything below
├── orders.json            Every online order in one array
├── receipts.json          Every receipt in one array, with line items
├── orders/
│   └── <order number>.json
└── receipts/
    └── <barcode>.json
```

`manifest.json` is the place to start — it holds the totals, the date range, any
records that could not be downloaded, and a one-line entry per file:

```json
{
  "library_version": "1.1.0",
  "generated_at": "2026-08-06T09:14:22Z",
  "since": "2016-08-06",
  "until": "2026-08-06",
  "order_count": 214,
  "receipt_count": 388,
  "item_count": 7431,
  "online_order_total": 41203.87,
  "warehouse_receipt_total": 76914.02,
  "receipts": [
    {
      "id": "21134300501862509051323",
      "date": "2026-08-01",
      "total": 269.13,
      "description": "MERIDIAN",
      "file": "receipts/21134300501862509051323.json"
    }
  ]
}
```

Each receipt file contains the full transaction: warehouse and address, every
line item with quantity and price, the tax breakdown, payment method, instant
savings and the membership number.

## Resuming and updating

Receipts are written to disk the moment they arrive, and every run skips
receipts that are already saved with their line items. That means:

- **An interrupted download costs nothing.** Press Ctrl-C, re-run the same
  command, and it picks up only what is missing.
- **Keeping the archive current is cheap.** Re-running later fetches only new
  receipts. Add `-since` to narrow the scan: `./costco-cli -since 2026-01-01`.
  `manifest.json`, `orders.json` and `receipts.json` always cover everything in
  the directory, not just the dates of the latest run.
- **Individual failures do not sink the run.** If one receipt's line items are
  unavailable, its summary is saved, it is listed at the end and the command
  exits non-zero; re-running retries just those. The same goes for receipts
  saved by a `-no-items` run: a later run without the flag fills in their line
  items. Use `-force` to re-download records that are already complete.

## CLI flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `-cmd` | `download` | `download`, `login`, `snippet`, `setup`, `import-token` or `info` |
| `-out` | `costco-history` | Directory to write into |
| `-since` | 10 years ago | Earliest date to download (`YYYY-MM-DD`) |
| `-until` | today | Latest date to download (`YYYY-MM-DD`) |
| `-window` | `365` | Maximum days requested per API call |
| `-page-size` | `50` | Online orders requested per page |
| `-delay` | `250ms` | Pause between API calls |
| `-retries` | `3` | Retries per failed API call |
| `-no-items` | off | Skip per-receipt line item lookups (much faster, less detail) |
| `-force` | off | Re-download receipts that are already saved |
| `-quiet` | off | Print only the final summary |
| `-browser-login` | off | Sign in only through the window the tool opens; never fall back to the console command |
| `-console-login` | off | Sign in only with the console command in your own browser; never open a window |
| `-temp-profile` | off | Give the sign-in window a throwaway profile, so no Costco cookies are kept on disk |
| `-browser` | auto-detect | Chrome, Edge, Chromium or Brave executable for the sign-in window |
| `-non-interactive` | off | Never ask for a sign-in; fail instead (for unattended runs) |

If a wide date range gets rejected by Costco's API, narrow the window:
`./costco-cli -window 90`.

## Signing in

Costco's sign-in has to happen in a browser. `costco-cli` gets you there in one
of two ways.

### The sign-in window (default)

`costco-cli` starts Chrome, Edge, Chromium or Brave with a profile of its own,
separate from your everyday one, and opens Costco's order history there, which
sends you to the sign-in page. Sign in with whatever you normally use. When
Costco hands the page its tokens, `costco-cli` reads that one response over the
DevTools protocol and closes the window.

The profile is kept in `~/.costco/browser-profile`, so the window still has
Costco's cookies the next time it opens. If Costco still accepts them, the
window signs in and closes without you typing anything; if not, you sign in
again. How long Costco honours those cookies is up to Costco.

- Passwords and passkeys saved only in your everyday browser profile are not
  available in this one. OS-level passkeys (iCloud Keychain, Windows Hello),
  security keys and phone sign-in work.
- `-temp-profile` uses a throwaway profile that is deleted when the window
  closes, so nothing is remembered and you sign in by hand every time.
- Delete `~/.costco/browser-profile` at any time to remove the cookies.
  `costco-cli` recreates the folder when it next needs it.

### The console command (fallback)

Used when no window can be opened (no supported browser, no display), when the
window fails, or when you pass `-console-login`. You sign in in your own
browser, the way you always do (password, passkey, security key or passcode).
Then:

1. Open **Orders & Returns**. That page holds the sign-in this tool needs.
2. Press **F12** (Mac: **Cmd+Option+J**; Safari: enable *Show features for web
   developers* in Settings → Advanced, then **Cmd+Option+C**) and open the
   **Console** tab.
3. Paste this command and press Enter. If the browser blocks the paste, type
   the words it asks for first ("allow pasting" in English).

```js
(async () => { const CLIENT = "a3a5186b-7c89-4b4c-93a8-dd604e930757", TOKEN_URL = "https://signin.costco.com/e0714dd4-784d-46d6-a278-3e29553483eb/b2c_1a_sso_wcs_signup_signin_209/oauth2/v2.0/token"; const toClipboard = typeof copy === "function" ? copy : null; const found = {}; let others = 0; for (const store of [sessionStorage, localStorage]) { for (let i = 0; i < store.length; i++) { let entry; try { entry = JSON.parse(store.getItem(store.key(i))); } catch (e) { continue; } if (!entry || !entry.secret || !/^(IdToken|RefreshToken)$/.test(entry.credentialType)) continue; if (entry.clientId !== CLIENT) { others++; continue; } found[entry.credentialType] = entry.secret; } } if (!found.RefreshToken) { console.error(others ? "This page has a sign-in for a different Costco app. Open Orders & Returns and run this again." : "No Costco sign-in on this page. Sign in, open Orders & Returns, then run this again."); return; } let tokens, response; try { response = await fetch(TOKEN_URL, {method: "POST", body: new URLSearchParams({client_id: CLIENT, grant_type: "refresh_token", refresh_token: found.RefreshToken})}); const text = await response.text(); try { tokens = JSON.parse(text); } catch (e) { tokens = {}; } } catch (e) { if (!found.IdToken) { console.error("Could not reach Costco: " + e); return; } console.warn("Could not reach Costco to refresh the sign-in; using the one on this page."); tokens = {id_token: found.IdToken, refresh_token: found.RefreshToken, refresh_token_expires_in: 7776000}; } if (response && (!response.ok || !tokens.refresh_token)) { console.error("Costco refused the sign-in (" + (tokens.error_description || tokens.error || response.status) + "). Sign out, sign in again and rerun this."); return; } const block = "-----BEGIN COSTCO SIGN-IN-----\n" + btoa(unescape(encodeURIComponent(JSON.stringify(tokens)))).match(/.{1,64}/g).join("\n") + "\n-----END COSTCO SIGN-IN-----"; if (toClipboard) toClipboard(block); console.log(block + "\n\n" + (toClipboard ? "Copied to your clipboard. " : "Copy everything from BEGIN to END. ") + "Now run costco-cli."); return block; })()
```

4. Run `./costco-cli` (or press Enter, if it is already waiting).

The command finds the sign-in that Costco's order history app keeps in the
page's storage. It asks Costco's own sign-in server for fresh tokens with it,
exactly as the app itself does, then prints them as a short block and copies
that block to your clipboard. `costco-cli` reads the block from the clipboard
(`pbpaste`, PowerShell `Get-Clipboard`, `wl-paste`, `xclip` or `xsel`), or you
can paste it into the terminal. The block is split into short lines because
macOS terminals cut a pasted line off at 1024 characters, and a token is
several KB. `./costco-cli -cmd snippet` prints the command whenever you need it.

If a sign-in block is already in the clipboard when `costco-cli` needs one, it
is used straight away and no window opens.

Either way, tokens then refresh automatically for about 90 days. When Costco
stops accepting them, the next run asks you to sign in again; a download
interrupted that way resumes where it stopped. `./costco-cli -cmd login` signs
in without downloading anything.

**Only paste console commands you trust.** "Paste this into the console" is a
classic way to steal accounts. This one reads the Costco sign-in on the page,
talks to `signin.costco.com` and nothing else, and copies the result to your
clipboard. You can watch its single request in the DevTools Network tab. Once
`costco-cli` has imported the sign-in, copy something else to clear it from the
clipboard.

### Other ways in

- **`./costco-cli -cmd import-token < token.json`** imports a saved block, or
  the raw JSON response of the `oauth2/v2.0/token` request copied from the
  DevTools Network tab.
- **Unattended runs** (cron and the like) should pass `-non-interactive`. They
  then fail with instructions instead of waiting for someone to sign in.

Run `./costco-cli -cmd info` at any time to see where config and tokens live and
whether they are still valid.

### Where your data goes

Nowhere but your own machine and Costco. The only hosts `costco-cli` itself
contacts are `signin.costco.com` (token refresh) and `ecom-api.costco.com` (the
GraphQL API); there is no telemetry or third-party reporting. The tool never
sees your password, passkey or security key, only OAuth tokens Costco issued.
Tokens are stored in `~/.costco/tokens.json` and downloaded history is written
with user-only permissions (`0600` files, `0700` directories), because receipts
contain your membership number, warehouse addresses and payment descriptions.

The sign-in window is a real browser, so it loads whatever costco.com loads.
Its profile, `~/.costco/browser-profile`, holds Costco's cookies and stays on
your computer: the folder is readable by your user only (`0700`), `costco-cli`
never copies or sends its contents anywhere, and the window is started with
`--disable-sync`, `--disable-breakpad`, `--metrics-recording-only` and
`--disable-background-networking`, so the browser does not sync the profile to
a Google or Microsoft account and does not upload crash reports or usage
statistics from it. The browser still downloads its own components into the
folder (certificate revocation lists and the like), which is why it grows to
roughly 150 MB. Anyone who can read that folder can act as you on costco.com
for as long as the cookies last, exactly as with your everyday browser profile;
pass `-temp-profile` if you would rather keep nothing.

The window is also started with
`--disable-blink-features=AutomationControlled`. Chrome otherwise marks any
window with a DevTools connection as automated (`navigator.webdriver`), and
sign-in pages may refuse such windows even though a person is signing in.

## Library usage

```go
package main

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

func main() {
	ctx := context.Background()

	// Only needed when no usable tokens are saved yet. Pipe in what the console
	// command (costco.ConsoleSnippet()) printed; costco.LoginWithBrowser returns
	// the same TokenResponse by opening a sign-in window instead. Its
	// BrowserLoginOptions.ProfileDir keeps the window's profile between
	// sign-ins; costco.BrowserProfileDir() is the folder the CLI uses.
	if tokens, _ := costco.LoadTokens(); tokens == nil || time.Now().After(tokens.RefreshTokenExpiresAt) {
		signInBlock, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		response, err := costco.ParseSignIn(string(signInBlock))
		if err != nil {
			log.Fatal(err)
		}
		saved, err := costco.ImportTokenResponse(response)
		if err != nil {
			log.Fatal(err)
		}
		if err := costco.SaveTokens(saved); err != nil {
			log.Fatal(err)
		}
	}

	client := costco.NewClient(costco.Config{
		WarehouseNumber:    "847",
		TokenRefreshBuffer: 5 * time.Minute,
	})

	store, err := costco.NewFileStore("costco-history")
	if err != nil {
		log.Fatal(err)
	}

	history, err := client.DownloadHistory(ctx, costco.HistoryOptions{
		Since:    time.Now().AddDate(-10, 0, 0),
		Store:    store,
		Progress: func(message string) { log.Println(message) },
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := store.WriteHistory(history); err != nil {
		log.Fatal(err)
	}

	log.Printf("%d orders, %d receipts, $%.2f in warehouse spending",
		len(history.Orders), len(history.Receipts), history.WarehouseReceiptTotal())
}
```

`HistoryOptions` controls the download:

| Field | Default | Purpose |
| --- | --- | --- |
| `Since` / `Until` | — / now | Inclusive date range |
| `WindowDays` | `365` | Maximum days per API call |
| `PageSize` | `50` | Online orders per page |
| `SkipReceiptDetails` | `false` | Keep receipt summaries, skip line items |
| `Store` | `nil` | Persist records as they arrive; enables resuming |
| `Force` | `false` | Ignore records already in `Store` |
| `RequestDelay` | `0` | Pause between API calls |
| `MaxRetries` | `2` | Extra attempts per failed call |
| `Progress` | `nil` | Receives human-readable status lines |

Anything the downloader could not retrieve lands in `History.Warnings` rather
than aborting the run. The exception is `ErrNotAuthenticated`: when Costco stops
accepting the sign-in, the download stops with that error. Sign in again and
call `DownloadHistory` with the same `Store`; it skips what is already saved.

The lower-level calls remain available if you want a single slice of data
instead of a full archive: `GetOnlineOrders`, `FetchAllOnlineOrders`,
`GetReceipts` and `GetReceiptDetail`.

### Logging

The client takes an optional `*slog.Logger`. With no logger, everything is
discarded silently.

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelDebug,
}))

client := costco.NewClient(costco.Config{
	WarehouseNumber: "847",
	Logger:          logger,
})
```

Tokens and credentials are never logged, at any level.

## Working with downloaded receipts

Costco returns discounts as separate line items rather than adjusting the item
they apply to. A discount item has a negative amount, a negative unit count, and
a description starting with `/` followed by the parent item number or
description (for example `/1553261`).

Returns also have negative amounts, but they carry a normal description and
appear on receipts with `TransactionType: "Refund"`.

```go
// Identify a discount and the item it belongs to.
for _, item := range receipt.ItemArray {
	if item.IsDiscount() {
		fmt.Printf("$%.2f off item %s\n", math.Abs(item.Amount), item.GetParentItemNumber())
	}
}

// Or apply every discount to its parent in one call.
netted, orphaned := costco.NetDiscounts(receipt.ItemArray)
```

`NetDiscounts` matches a discount to its parent by item number, then by exact
description, then by substring, and finally by word overlap — which handles
coupons that reference a product generically (`/AAA BATTERY` against
`DURACELL AAA`). Discounts it cannot place are returned as `orphaned`.

The discount amount is already included in the receipt's `SubTotal`, so do not
subtract it a second time.

## API details

- **Auth endpoint**: `https://signin.costco.com/.../oauth2/v2.0/token`
- **GraphQL endpoint**: `https://ecom-api.costco.com/ebusiness/order/v1/orders/graphql`
- **Auth header**: `costco-x-authorization: Bearer {id_token}`

The client refreshes tokens before they expire, manages them in a thread-safe
way, and persists them to `~/.costco/tokens.json`.

## Running tests

```bash
go test ./... -v
```

The browser tests launch a real Chrome/Edge and skip themselves when none is
installed; `go test -short ./...` skips them explicitly.

## Running in a container

The token file is all a download needs, so a container can run with just that
file mounted. Sign in on your own machine first (see
[Signing in](#signing-in)); nobody can answer a sign-in prompt inside the
container, so it runs with `-non-interactive` and stops with instructions when
the token needs renewing.

```bash
# Once: build the image straight from GitHub (no clone needed).
docker build -t costco-cli "https://github.com/ssjfrank/costco-go.git#cursor/download-full-order-history-9b26"

# Each download: mount the token file and an output directory.
mkdir -p "$HOME/costco"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --mount type=bind,source="$HOME/.costco/tokens.json",target=/secrets/tokens.json \
  -v "$HOME/costco:/data" \
  costco-cli
```

The history lands in `~/costco/costco-history`. Extra flags go at the end, for
example `costco-cli -since 2026-01-01`.

- **Where the token file lives.** Signing in writes `~/.costco/tokens.json`.
  To keep it somewhere else, set `COSTCO_TOKEN_FILE` when signing in, and
  mount that path instead:
  `COSTCO_TOKEN_FILE="$HOME/costco/tokens.json" ./costco-cli -cmd login`.
  Inside the image, `COSTCO_TOKEN_FILE` is `/secrets/tokens.json`.
- **Mount the file read-write.** Every token refresh writes a new refresh
  token back to the file, in place, which a single-file mount allows. A
  `readonly` mount still works, but the refreshed token is lost when the
  container exits. The saved refresh token then ages until it expires.
- **Use `--mount`, not `-v`, for the token file.** If the file does not exist
  yet, `-v` silently creates a directory with that name on your machine, and
  that directory breaks later sign-ins until you delete it. `--mount` refuses
  to start instead. The program recognises the directory and says so.
- **`--user "$(id -u):$(id -g)"`** lets the container read the owner-only
  token file on Linux and makes the downloaded files yours. On Docker Desktop
  for Mac it does no harm.
- The image is built from `gcr.io/distroless/static-debian12:nonroot`: the
  static binary, CA certificates, and nothing else (about 16 MB). The build
  context excludes `costco-history/`, `token.json` and `.costco/`, so personal
  data never ends up in an image.

## Publishing binaries

Every CI run (each pull request and push to `main`) builds the same binaries
and attaches them to the run: open the run under **Actions** and download
`costco-cli-1.1.0-windows-amd64.exe`, `costco-cli-1.1.0-linux-amd64` and the
rest from its **Artifacts** section. GitHub delivers each one as a zip and keeps it for 90
days. The `Build` jobs also start the Windows, Linux and Apple Silicon binaries
on matching machines to check that they run.

Pushing a version tag publishes a release. `.github/workflows/release.yml` runs
the tests, builds macOS (Apple Silicon and Intel), Linux and Windows binaries
with `scripts/build-release.sh`, and attaches them to a GitHub Release along
with `costco-cli-<version>-SHA256SUMS.txt`. Tags with a suffix, such as
`v1.1.0-rc.1`, become pre-releases.

Every file is named after the `Version` in `pkg/costco/constants.go`
(`costco-cli-1.1.0-linux-amd64`, `costco-cli-1.1.0-windows-amd64.exe`, ...),
and `costco-cli -version` prints that version and the commit it was built from.
A release is refused when its tag is not `v` followed by that version, so a
tag can never publish files that claim another version.

```bash
git tag v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

The binaries are cross-compiled with CGO disabled. Go's linker ad-hoc signs
darwin/arm64 binaries itself, which Apple Silicon requires before it will run
them, so no Mac is needed to build. On a fork, enable GitHub Actions (the
**Actions** tab) before pushing the tag; a tag pushed while Actions is off
publishes nothing.

## License

MIT
