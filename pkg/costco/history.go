package costco

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

const (
	// defaultWindowDays is the widest date range requested in a single API call.
	// Costco rejects or truncates very wide ranges, so history is walked in chunks.
	defaultWindowDays = 365

	// defaultHistoryPageSize is the number of online orders requested per page.
	defaultHistoryPageSize = 50

	// maxOrderPages guards against a server that keeps reporting more records
	// than it returns, which would otherwise loop forever.
	maxOrderPages = 500
)

// DateWindow is an inclusive date range used to walk history in chunks.
type DateWindow struct {
	Start time.Time
	End   time.Time
}

// StartDate returns the window start in the YYYY-MM-DD format used by the orders API.
func (w DateWindow) StartDate() string { return w.Start.Format("2006-01-02") }

// EndDate returns the window end in the YYYY-MM-DD format used by the orders API.
func (w DateWindow) EndDate() string { return w.End.Format("2006-01-02") }

// String renders the window as "start to end" for logs and progress output.
func (w DateWindow) String() string { return w.StartDate() + " to " + w.EndDate() }

// SplitDateRange divides the inclusive range [since, until] into contiguous,
// non-overlapping windows of at most windowDays days each, newest window first.
// A non-positive windowDays falls back to one year. An empty slice is returned
// when until is before since.
func SplitDateRange(since, until time.Time, windowDays int) []DateWindow {
	if windowDays <= 0 {
		windowDays = defaultWindowDays
	}

	since = truncateToDay(since)
	until = truncateToDay(until)
	if until.Before(since) {
		return nil
	}

	var windows []DateWindow
	for end := until; !end.Before(since); {
		start := end.AddDate(0, 0, -(windowDays - 1))
		if start.Before(since) {
			start = since
		}
		windows = append(windows, DateWindow{Start: start, End: end})
		end = start.AddDate(0, 0, -1)
	}

	return windows
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// normalizeReceiptDate converts a YYYY-MM-DD date into the M/DD/YYYY format that
// Costco's receipt queries require. Values in any other format are passed through.
func normalizeReceiptDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return fmt.Sprintf("%d/%02d/%d", int(parsed.Month()), parsed.Day(), parsed.Year())
}

// HistoryStore persists records as they are downloaded. Supplying a store makes a
// download resumable: receipts already present are read back from storage instead
// of being fetched again.
type HistoryStore interface {
	SaveOrder(order *OnlineOrder) error
	SaveReceipt(receipt *Receipt) error
	HasReceipt(barcode string) bool
	LoadReceipt(barcode string) (*Receipt, error)
}

// HistoryOptions configures a full-history download.
type HistoryOptions struct {
	// Since and Until bound the download. Both are inclusive.
	Since time.Time
	Until time.Time

	// WindowDays is the widest date range requested per API call (default 365).
	WindowDays int

	// PageSize is the number of online orders requested per page (default 50).
	PageSize int

	// SkipReceiptDetails keeps the fast receipt summaries and skips the extra
	// per-receipt call that fetches line items.
	SkipReceiptDetails bool

	// Store, when set, receives every record as it is downloaded and allows an
	// interrupted download to resume where it left off.
	Store HistoryStore

	// Force re-fetches receipts that are already present in Store.
	Force bool

	// RequestDelay is paused between API calls to stay friendly to Costco's API.
	RequestDelay time.Duration

	// MaxRetries is the number of extra attempts for a failing call (default 2).
	MaxRetries int

	// RetryDelay is the base backoff between retries; it doubles each attempt.
	RetryDelay time.Duration

	// Progress, when set, receives human-readable status lines.
	Progress func(message string)
}

func (o HistoryOptions) withDefaults() HistoryOptions {
	if o.WindowDays <= 0 {
		o.WindowDays = defaultWindowDays
	}
	if o.PageSize <= 0 {
		o.PageSize = defaultHistoryPageSize
	}
	if o.MaxRetries == 0 {
		o.MaxRetries = 2
	}
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
	}
	if o.Until.IsZero() {
		o.Until = time.Now()
	}
	return o
}

func (o HistoryOptions) report(message string) {
	if o.Progress != nil {
		o.Progress(message)
	}
}

// pause waits out RequestDelay, returning early if the context is cancelled.
func (o HistoryOptions) pause(ctx context.Context) error {
	if o.RequestDelay <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(o.RequestDelay):
		return nil
	}
}

// History is the complete set of records downloaded for an account.
type History struct {
	Since       time.Time     `json:"since"`
	Until       time.Time     `json:"until"`
	GeneratedAt time.Time     `json:"generated_at"`
	Orders      []OnlineOrder `json:"orders"`
	Receipts    []Receipt     `json:"receipts"`

	// Warnings lists records that could not be downloaded. Re-running the
	// download with the same output directory retries only those records.
	Warnings []string `json:"warnings,omitempty"`
}

// ItemCount returns the total number of receipt line items across all receipts.
func (h *History) ItemCount() int {
	total := 0
	for i := range h.Receipts {
		total += len(h.Receipts[i].ItemArray)
	}
	return total
}

// OnlineOrderTotal returns the summed value of every downloaded online order.
func (h *History) OnlineOrderTotal() float64 {
	total := 0.0
	for i := range h.Orders {
		total += h.Orders[i].OrderTotal
	}
	return total
}

// WarehouseReceiptTotal returns the summed value of every downloaded receipt.
func (h *History) WarehouseReceiptTotal() float64 {
	total := 0.0
	for i := range h.Receipts {
		total += h.Receipts[i].Total
	}
	return total
}

func (h *History) warn(message string) {
	h.Warnings = append(h.Warnings, message)
}

// DownloadHistory downloads every online order and warehouse receipt between
// opts.Since and opts.Until. The range is walked newest-first in windows small
// enough for Costco's API, online orders are paged until exhausted, and each
// receipt is expanded into its full line-item detail.
//
// A failure in the newest window aborts the download, because that almost always
// means the stored tokens are no longer valid. Later failures are collected in
// History.Warnings so that a single unavailable record cannot discard years of
// successfully downloaded data; re-running the download retries them.
//
// Example:
//
//	store, _ := costco.NewFileStore("costco-history")
//	history, err := client.DownloadHistory(ctx, costco.HistoryOptions{
//	    Since: time.Now().AddDate(-10, 0, 0),
//	    Store: store,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	_ = store.WriteHistory(history)
func (c *Client) DownloadHistory(ctx context.Context, opts HistoryOptions) (*History, error) {
	opts = opts.withDefaults()

	if opts.Until.Before(opts.Since) {
		return nil, fmt.Errorf("start date %s is after end date %s",
			opts.Since.Format("2006-01-02"), opts.Until.Format("2006-01-02"))
	}

	windows := SplitDateRange(opts.Since, opts.Until, opts.WindowDays)
	if len(windows) == 0 {
		return nil, fmt.Errorf("date range %s to %s contains no days",
			opts.Since.Format("2006-01-02"), opts.Until.Format("2006-01-02"))
	}

	history := &History{
		Since:       truncateToDay(opts.Since),
		Until:       truncateToDay(opts.Until),
		GeneratedAt: time.Now(),
	}

	c.getLogger().Info("downloading full history",
		slog.String("since", history.Since.Format("2006-01-02")),
		slog.String("until", history.Until.Format("2006-01-02")),
		slog.Int("windows", len(windows)))

	seenOrders := make(map[string]bool)
	seenReceipts := make(map[string]bool)

	for i, window := range windows {
		label := fmt.Sprintf("[%d/%d] %s", i+1, len(windows), window)
		newest := i == 0

		opts.report(label + ": online orders")
		orders, err := c.downloadOrderWindow(ctx, window, opts)
		if err != nil {
			if newest {
				return nil, fmt.Errorf("fetching online orders for %s: %w", window, err)
			}
			history.warn(fmt.Sprintf("online orders for %s: %v", window, err))
		}
		added := c.collectOrders(history, orders, seenOrders, opts)
		opts.report(fmt.Sprintf("%s: %d online order(s)", label, added))

		opts.report(label + ": warehouse receipts")
		receipts, err := c.downloadReceiptWindow(ctx, window, opts)
		if err != nil {
			if newest {
				return nil, fmt.Errorf("fetching receipts for %s: %w", window, err)
			}
			history.warn(fmt.Sprintf("receipts for %s: %v", window, err))
		}
		added, err = c.collectReceipts(ctx, history, receipts, seenReceipts, opts)
		if err != nil {
			return nil, err
		}
		opts.report(fmt.Sprintf("%s: %d receipt(s)", label, added))
	}

	sortOrdersNewestFirst(history.Orders)
	sortReceiptsNewestFirst(history.Receipts)

	c.getLogger().Info("history download complete",
		slog.Int("orders", len(history.Orders)),
		slog.Int("receipts", len(history.Receipts)),
		slog.Int("warnings", len(history.Warnings)))

	return history, nil
}

func (c *Client) collectOrders(history *History, orders []OnlineOrder, seen map[string]bool, opts HistoryOptions) int {
	added := 0
	for i := range orders {
		order := orders[i]
		key := order.OrderNumber
		if key == "" {
			key = order.OrderHeaderID
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true

		if opts.Store != nil {
			if err := opts.Store.SaveOrder(&order); err != nil {
				history.warn(fmt.Sprintf("saving order %s: %v", key, err))
			}
		}
		history.Orders = append(history.Orders, order)
		added++
	}
	return added
}

// collectReceipts expands each receipt summary into full detail and records it.
// The returned error is reserved for cancellation; per-receipt problems become warnings.
func (c *Client) collectReceipts(ctx context.Context, history *History, receipts []Receipt, seen map[string]bool, opts HistoryOptions) (int, error) {
	added := 0
	for i := range receipts {
		summary := receipts[i]
		barcode := summary.TransactionBarcode
		if barcode == "" || seen[barcode] {
			continue
		}
		seen[barcode] = true

		if opts.Store != nil && !opts.Force && opts.Store.HasReceipt(barcode) {
			if stored, err := opts.Store.LoadReceipt(barcode); err == nil {
				history.Receipts = append(history.Receipts, *stored)
				added++
				continue
			}
		}

		receipt := summary
		if !opts.SkipReceiptDetails {
			detail, err := c.downloadReceiptDetail(ctx, summary, opts)
			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return added, err
			case err != nil:
				history.warn(fmt.Sprintf("receipt %s: %v", barcode, err))
			default:
				receipt = *detail
			}
		}

		if opts.Store != nil {
			if err := opts.Store.SaveReceipt(&receipt); err != nil {
				history.warn(fmt.Sprintf("saving receipt %s: %v", barcode, err))
			}
		}
		history.Receipts = append(history.Receipts, receipt)
		added++
	}
	return added, nil
}

func (c *Client) downloadOrderWindow(ctx context.Context, window DateWindow, opts HistoryOptions) ([]OnlineOrder, error) {
	var orders []OnlineOrder
	err := withRetries(ctx, opts.MaxRetries, opts.RetryDelay, func() error {
		var fetchErr error
		orders, fetchErr = c.FetchAllOnlineOrders(ctx, window.StartDate(), window.EndDate(), opts.PageSize)
		return fetchErr
	})
	if pauseErr := opts.pause(ctx); pauseErr != nil {
		return orders, pauseErr
	}
	return orders, err
}

func (c *Client) downloadReceiptWindow(ctx context.Context, window DateWindow, opts HistoryOptions) ([]Receipt, error) {
	var receipts []Receipt
	err := withRetries(ctx, opts.MaxRetries, opts.RetryDelay, func() error {
		response, fetchErr := c.GetReceipts(ctx, window.StartDate(), window.EndDate(), "all", "all")
		if fetchErr != nil {
			return fetchErr
		}
		receipts = response.Receipts
		return nil
	})
	if pauseErr := opts.pause(ctx); pauseErr != nil {
		return receipts, pauseErr
	}
	return receipts, err
}

func (c *Client) downloadReceiptDetail(ctx context.Context, summary Receipt, opts HistoryOptions) (*Receipt, error) {
	documentType := "warehouse"
	if summary.DocumentType == "fuel" || summary.ReceiptType == "Gas Station" {
		documentType = "fuel"
	}

	var detail *Receipt
	err := withRetries(ctx, opts.MaxRetries, opts.RetryDelay, func() error {
		var fetchErr error
		detail, fetchErr = c.GetReceiptDetail(ctx, summary.TransactionBarcode, documentType)
		return fetchErr
	})
	if pauseErr := opts.pause(ctx); pauseErr != nil {
		return detail, pauseErr
	}
	return detail, err
}

// FetchAllOnlineOrders pages through every online order between startDate and
// endDate (both YYYY-MM-DD) and returns them in the order the API reports them.
func (c *Client) FetchAllOnlineOrders(ctx context.Context, startDate, endDate string, pageSize int) ([]OnlineOrder, error) {
	if pageSize <= 0 {
		pageSize = defaultHistoryPageSize
	}

	var orders []OnlineOrder
	for page := 1; page <= maxOrderPages; page++ {
		response, err := c.GetOnlineOrders(ctx, startDate, endDate, page, pageSize)
		if err != nil {
			// An empty window reports no data at all rather than an empty page.
			if errors.Is(err, ErrNoOrderData) {
				return orders, nil
			}
			return nil, err
		}

		orders = append(orders, response.BCOrders...)
		if len(response.BCOrders) == 0 || len(orders) >= response.TotalNumberOfRecords {
			break
		}
	}

	return orders, nil
}

// withRetries runs fn, retrying up to retries extra times with exponential backoff.
func withRetries(ctx context.Context, retries int, delay time.Duration, fn func() error) error {
	attempts := retries + 1
	if attempts < 1 {
		attempts = 1
	}

	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if attempt == attempts {
			break
		}
		wait := delay * time.Duration(1<<(attempt-1))
		if wait <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return err
}

func sortOrdersNewestFirst(orders []OnlineOrder) {
	sort.SliceStable(orders, func(i, j int) bool {
		return orders[i].OrderPlacedDate > orders[j].OrderPlacedDate
	})
}

func sortReceiptsNewestFirst(receipts []Receipt) {
	sort.SliceStable(receipts, func(i, j int) bool {
		return receipts[i].TransactionDateTime > receipts[j].TransactionDateTime
	})
}
