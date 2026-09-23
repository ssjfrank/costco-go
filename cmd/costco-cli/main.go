package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

func main() {
	var (
		command     = flag.String("cmd", "download", "Command: download, login, setup, import-token, info")
		outputDir   = flag.String("out", "costco-history", "Directory to write the downloaded history into")
		since       = flag.String("since", "", "Earliest date to download (YYYY-MM-DD, default: 10 years ago)")
		until       = flag.String("until", "", "Latest date to download (YYYY-MM-DD, default: today)")
		windowDays  = flag.Int("window", 365, "Maximum number of days requested per API call")
		pageSize    = flag.Int("page-size", 50, "Number of online orders requested per page")
		delay       = flag.Duration("delay", 250*time.Millisecond, "Pause between API calls")
		retries     = flag.Int("retries", 3, "Retries per failed API call")
		skipDetails = flag.Bool("no-items", false, "Skip per-receipt line item lookups (faster, less detail)")
		force       = flag.Bool("force", false, "Re-download receipts that were already saved")
		quiet       = flag.Bool("quiet", false, "Print only the final summary")
		browserPath = flag.String("browser", "", "Chrome, Edge, Chromium or Brave executable for signing in (default: auto-detect)")
		noBrowser   = flag.Bool("no-browser", false, "Never open a sign-in window; fail instead (for unattended runs)")
	)

	flag.Usage = usage
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch *command {
	case "login":
		exitOnError(signInWithBrowser(ctx, *browserPath, os.Stdout))
	case "setup":
		exitOnError(setupCredentials())
	case "import-token":
		exitOnError(runImportTokens())
	case "info":
		fmt.Print(costco.GetConfigInfo())
	case "download":
		err := runDownload(ctx, downloadConfig{
			OutputDir:   *outputDir,
			Since:       *since,
			Until:       *until,
			WindowDays:  *windowDays,
			PageSize:    *pageSize,
			Delay:       *delay,
			Retries:     *retries,
			SkipDetails: *skipDetails,
			Force:       *force,
			Quiet:       *quiet,
			BrowserPath: *browserPath,
			NoBrowser:   *noBrowser,
		}, os.Stdout)

		// An incomplete download has already reported itself in the summary; the
		// non-zero exit is there so scripts notice the gap.
		if errors.Is(err, errIncompleteHistory) {
			os.Exit(1)
		}
		exitOnError(err)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", *command)
		usage()
		os.Exit(2)
	}
}

func exitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `costco-cli downloads your complete Costco purchase history to local JSON files.

  costco-cli                     Download everything into ./costco-history

The first time, a browser window opens on costco.com. Sign in there with any
method you normally use (password, passkey, security key); the window closes by
itself and the download starts. The sign-in is reused for about 90 days, and the
window reappears automatically when it runs out.

More:
  costco-cli -out ~/costco       Download into a different directory
  costco-cli -since 2020-01-01   Limit how far back to reach
  costco-cli -no-items           Skip receipt line items for a quick pass
  costco-cli -cmd login          Sign in again without downloading
  costco-cli -cmd info           Show where config and tokens are stored
  costco-cli -cmd setup          Set a warehouse number other than the default
  costco-cli -cmd import-token   Sign in by pasting a token copied from DevTools,
                                 for machines without Chrome or Edge

Downloads resume: re-running skips receipts already on disk, so an interrupted
run only fetches what is missing. Use -force to re-download everything.

Flags:
`)
	flag.PrintDefaults()
}
