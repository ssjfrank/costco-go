package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

// defaultHistoryYears is how far back the download reaches when no start date is
// given. Costco keeps warehouse receipts for a couple of years and online orders
// for longer, so asking for a decade simply returns everything that still exists.
const defaultHistoryYears = 10

// errIncompleteHistory reports that the download finished but some records were
// missing, so the process should exit non-zero even though data was written.
var errIncompleteHistory = errors.New("history is incomplete")

// downloadConfig holds the command line settings for a history download.
type downloadConfig struct {
	OutputDir   string
	Since       string
	Until       string
	WindowDays  int
	PageSize    int
	Delay       time.Duration
	Retries     int
	SkipDetails bool
	Force       bool
	Quiet       bool
}

// historyOptions resolves the configured date strings against now and converts
// the settings into library options.
func (c downloadConfig) historyOptions(now time.Time) (costco.HistoryOptions, error) {
	options := costco.HistoryOptions{
		WindowDays:         c.WindowDays,
		PageSize:           c.PageSize,
		RequestDelay:       c.Delay,
		MaxRetries:         c.Retries,
		SkipReceiptDetails: c.SkipDetails,
		Force:              c.Force,
	}

	until := now
	if c.Until != "" {
		parsed, err := time.Parse("2006-01-02", c.Until)
		if err != nil {
			return options, fmt.Errorf("-until must be a YYYY-MM-DD date, got %q", c.Until)
		}
		until = parsed
	}

	since := until.AddDate(-defaultHistoryYears, 0, 0)
	if c.Since != "" {
		parsed, err := time.Parse("2006-01-02", c.Since)
		if err != nil {
			return options, fmt.Errorf("-since must be a YYYY-MM-DD date, got %q", c.Since)
		}
		since = parsed
	}

	if until.Before(since) {
		return options, fmt.Errorf("-since %s must be before -until %s",
			since.Format("2006-01-02"), until.Format("2006-01-02"))
	}

	options.Since = since
	options.Until = until
	return options, nil
}

// runDownload downloads the account's entire history into cfg.OutputDir.
func runDownload(ctx context.Context, cfg downloadConfig, out io.Writer) error {
	options, err := cfg.historyOptions(time.Now())
	if err != nil {
		return err
	}

	tokens, err := costco.LoadTokens()
	if err != nil {
		return fmt.Errorf("loading saved tokens: %w", err)
	}
	if tokens == nil || time.Now().After(tokens.RefreshTokenExpiresAt) {
		return errors.New("no valid tokens found. Run 'costco-cli -cmd import-token' to import tokens from your browser")
	}

	stored, err := costco.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading saved config: %w", err)
	}
	clientConfig := costco.Config{
		WarehouseNumber:    costco.DefaultWarehouse,
		TokenRefreshBuffer: 5 * time.Minute,
	}
	if stored != nil {
		clientConfig.Email = stored.Email
		if stored.WarehouseNumber != "" {
			clientConfig.WarehouseNumber = stored.WarehouseNumber
		}
	}

	store, err := costco.NewFileStore(cfg.OutputDir)
	if err != nil {
		return err
	}
	options.Store = store
	if !cfg.Quiet {
		options.Progress = func(message string) { fmt.Fprintln(out, message) }
		fmt.Fprintf(out, "Downloading Costco history from %s to %s into %s\n\n",
			options.Since.Format("2006-01-02"), options.Until.Format("2006-01-02"), store.Dir())
	}

	history, err := costco.NewClient(clientConfig).DownloadHistory(ctx, options)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("interrupted. Everything downloaded so far is saved in %s; re-run the same command to finish", store.Dir())
		}
		return err
	}

	if err := store.WriteHistory(history); err != nil {
		return err
	}

	printSummary(out, history, store.Dir())

	if len(history.Warnings) > 0 {
		return errIncompleteHistory
	}
	return nil
}

func printSummary(out io.Writer, history *costco.History, outputDir string) {
	fmt.Fprintf(out, "\nDownloaded %s and %s (%s)\n",
		plural(len(history.Orders), "online order"),
		plural(len(history.Receipts), "warehouse receipt"),
		plural(history.ItemCount(), "receipt line item"))

	if !history.Since.IsZero() {
		fmt.Fprintf(out, "  Date range:          %s to %s\n",
			history.Since.Format("2006-01-02"), history.Until.Format("2006-01-02"))
	}
	fmt.Fprintf(out, "  Online order total:  $%.2f\n", history.OnlineOrderTotal())
	fmt.Fprintf(out, "  Receipt total:       $%.2f\n", history.WarehouseReceiptTotal())
	fmt.Fprintf(out, "  Saved to:            %s\n", outputDir)
	fmt.Fprintf(out, "  Start here:          %s\n", outputDir+"/manifest.json")

	if len(history.Warnings) == 0 {
		return
	}

	fmt.Fprintf(out, "\n%s could not be downloaded. Re-run the same command to retry just these:\n",
		plural(len(history.Warnings), "record"))
	for _, warning := range history.Warnings {
		fmt.Fprintf(out, "  - %s\n", warning)
	}
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
