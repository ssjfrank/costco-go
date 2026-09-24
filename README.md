# Costco Go Client

[![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)](https://github.com/eshaffer321/costco-go/releases/tag/v1.0.0)

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
curl -fL -o costco-cli "https://github.com/ssjfrank/costco-go/releases/latest/download/costco-cli-darwin-$ARCH"
chmod +x costco-cli
```

   Linux and Windows builds are on the
   [Releases](https://github.com/ssjfrank/costco-go/releases) page, or build from
   source with Go 1.24+: `go build -o costco-cli ./cmd/costco-cli`.
2. In your usual browser, sign in at [costco.com](https://www.costco.com) and
   open **Orders & Returns**.
3. Press **F12** (Mac: **Cmd+Option+J**), open the **Console**, paste the
   command from [Signing in](#signing-in) and press Enter. It copies your
   sign-in to the clipboard.
4. Run it:

```bash
./costco-cli
```

It picks the sign-in up from the clipboard and starts downloading:

```
You are not signed in to Costco yet, or your last sign-in has expired.
Found a Costco sign-in in your clipboard.
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
`./costco-history`. Later runs reuse the sign-in for about 90 days. When it
runs out, `costco-cli` shows the steps and the command again and waits for you
to press Enter. Running `costco-cli` first and doing the browser steps while it
waits works just as well.

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
  "library_version": "1.0.0",
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
receipts that are already saved. That means:

- **An interrupted download costs nothing.** Press Ctrl-C, re-run the same
  command, and it picks up only what is missing.
- **Keeping the archive current is cheap.** Re-running later fetches only new
  receipts. Add `-since` to narrow the scan: `./costco-cli -since 2026-01-01`.
- **Individual failures do not sink the run.** If one receipt is unavailable,
  it is listed at the end and the command exits non-zero; re-running retries
  just those. Use `-force` to re-download records that are already saved.

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
| `-browser-login` | off | Sign in through a browser window the tool opens, instead of the console command |
| `-browser` | auto-detect | Chrome, Edge, Chromium or Brave executable for `-browser-login` |
| `-non-interactive` | off | Never ask for a sign-in; fail instead (for unattended runs) |

If a wide date range gets rejected by Costco's API, narrow the window:
`./costco-cli -window 90`.

## Signing in

Costco's sign-in has to happen in a browser, so you do it in yours, the way you
always do (password, passkey, security key or passcode). Then:

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

After that, tokens refresh automatically for about 90 days. When Costco stops
accepting them, the next run shows the steps again and waits; a download
interrupted that way resumes where it stopped. `./costco-cli -cmd login` signs
in without downloading anything.

**Only paste console commands you trust.** "Paste this into the console" is a
classic way to steal accounts. This one reads the Costco sign-in on the page,
talks to `signin.costco.com` and nothing else, and copies the result to your
clipboard. You can watch its single request in the DevTools Network tab. Once
`costco-cli` has imported the sign-in, copy something else to clear it from the
clipboard.

Other ways in:

- **`./costco-cli -browser-login`** starts Chrome, Edge, Chromium or Brave with
  a fresh temporary profile, opens Costco's order history there (which sends you
  to the sign-in page), and captures the token response over the DevTools
  protocol when you land back. The window then closes and the profile is
  deleted. Passwords and passkeys saved only in your usual browser profile are
  not available in the temporary one; OS-level passkeys (iCloud Keychain,
  Windows Hello), security keys and phone sign-in work.
- **`./costco-cli -cmd import-token < token.json`** imports a saved block, or
  the raw JSON response of the `oauth2/v2.0/token` request copied from the
  DevTools Network tab.
- **Unattended runs** (cron and the like) should pass `-non-interactive`. They
  then fail with instructions instead of waiting for someone to sign in.

Run `./costco-cli -cmd info` at any time to see where config and tokens live and
whether they are still valid.

### Where your data goes

Nowhere but your own machine and Costco. The only hosts contacted are
`signin.costco.com` (token refresh) and `ecom-api.costco.com` (the GraphQL API),
plus whatever costco.com loads in a `-browser-login` window; there is no
telemetry or third-party reporting. The tool never sees your password, passkey
or security key, only OAuth tokens Costco issued. Tokens are stored in
`~/.costco/tokens.json` and downloaded history is written with user-only
permissions (`0600` files, `0700` directories), because receipts contain your
membership number, warehouse addresses and payment descriptions.

The `-browser-login` window is started with
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
	// the same TokenResponse by opening a sign-in window instead.
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

## Publishing binaries

Pushing a version tag publishes a release. `.github/workflows/release.yml` runs
the tests, builds macOS (Apple Silicon and Intel), Linux and Windows binaries
with `scripts/build-release.sh`, and attaches them to a GitHub Release along
with `SHA256SUMS`. Tags with a suffix, such as `v1.0.0-rc.1`, become
pre-releases.

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
