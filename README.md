# Costco Go Client

[![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)](https://github.com/eshaffer321/costco-go/releases/tag/v1.0.0)

Download your complete Costco purchase history — every online order and every
warehouse receipt, down to the individual line items — into local JSON files.

Costco's website only shows a few months at a time behind a date picker. This
tool walks the entire history for you and writes it to disk, so you can search,
analyse or archive years of purchases offline.

## Quick start

中文用户请看分步指南：[docs/GUIDE.zh-CN.md](docs/GUIDE.zh-CN.md)

```bash
go build -o costco-cli ./cmd/costco-cli
./costco-cli
```

The first time, a browser window opens on Costco's sign-in page. Sign in there
the way you normally do — password, passkey, security key or passcode. The
window closes by itself and the download starts:

```
You are not signed in to Costco yet, or your last sign-in has expired.
A browser window has opened on Costco's sign-in page. Sign in there with any method you normally use;
the window closes by itself once your order history starts loading.
✓ Signed in. You won't need to sign in again until about 2026-12-22.

Downloading Costco history from 2016-09-23 to 2026-09-23 into costco-history

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
`./costco-history`. Later runs reuse the sign-in for about 90 days; after that
the window simply appears again.

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
| `-cmd` | `download` | `download`, `login`, `setup`, `import-token` or `info` |
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
| `-browser` | auto-detect | Chrome, Edge, Chromium or Brave executable for signing in |
| `-no-browser` | off | Never open a sign-in window; fail instead (for unattended runs) |

If a wide date range gets rejected by Costco's API, narrow the window:
`./costco-cli -window 90`.

## Signing in

Costco's sign-in has to happen in a browser, so the tool borrows one. When it
needs a sign-in, it starts your installed Chrome, Edge, Chromium or Brave with a
fresh, temporary profile and opens Costco's order history there. Being signed
out, that sends you straight to the sign-in page. You sign in by hand with
whatever method your account uses. When you land back in your order history,
costco.com requests its OAuth tokens. The tool reads that one response through
the browser's DevTools connection, saves it, closes the window and deletes the
temporary profile.

After that, tokens refresh automatically for about 90 days. When Costco stops
accepting them, the next run opens the window again; a download interrupted
that way resumes where it stopped. `./costco-cli -cmd login` signs in without
downloading anything.

A few things to know:

- **Your own browser profile is never touched.** Saved passwords and passkeys
  that live only inside your usual Chrome profile are not available in the
  temporary one. Passkeys stored by the operating system (iCloud Keychain,
  Windows Hello), security keys and phone-based passkeys all work.
- **Unattended runs** (cron and the like) should pass `-no-browser`. They then
  fail with instructions instead of waiting for someone to sign in.
- **No supported browser?** Copy the token by hand: sign in to costco.com with
  DevTools open (Network tab, **Preserve log** on), open **Orders & Returns**,
  select the `POST` request whose URL ends in `oauth2/v2.0/token`, copy its
  **Response**, save it as `token.json`, then run
  `./costco-cli -cmd import-token < token.json` and delete the file.

Run `./costco-cli -cmd info` at any time to see where config and tokens live and
whether they are still valid.

### Where your data goes

Nowhere but your own machine and Costco. The only hosts contacted are
`signin.costco.com` (token refresh) and `ecom-api.costco.com` (the GraphQL API),
plus whatever costco.com loads in the sign-in window; there is no telemetry or
third-party reporting. The tool never sees your password, passkey or security
key — only the token response costco.com itself receives. Tokens are stored in
`~/.costco/tokens.json` and downloaded history is written with user-only
permissions (`0600` files, `0700` directories), because receipts contain your
membership number, warehouse addresses and payment descriptions.

The sign-in window is started with
`--disable-blink-features=AutomationControlled`. Chrome otherwise marks any
window with a DevTools connection as automated (`navigator.webdriver`), and
sign-in pages may refuse such windows even though a person is signing in.

## Library usage

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

func main() {
	ctx := context.Background()

	// Only needed when no usable tokens are saved yet. Opens a browser window
	// and waits for the user to sign in there.
	if tokens, _ := costco.LoadTokens(); tokens == nil || time.Now().After(tokens.RefreshTokenExpiresAt) {
		response, err := costco.LoginWithBrowser(ctx, costco.BrowserLoginOptions{})
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

## License

MIT
