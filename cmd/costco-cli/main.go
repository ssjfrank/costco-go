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
		command        = flag.String("cmd", "download", "Command: download, login, snippet, setup, import-token, info")
		outputDir      = flag.String("out", "costco-history", "Directory to write the downloaded history into")
		since          = flag.String("since", "", "Earliest date to download (YYYY-MM-DD, default: 10 years ago)")
		until          = flag.String("until", "", "Latest date to download (YYYY-MM-DD, default: today)")
		windowDays     = flag.Int("window", 365, "Maximum number of days requested per API call")
		pageSize       = flag.Int("page-size", 50, "Number of online orders requested per page")
		delay          = flag.Duration("delay", 250*time.Millisecond, "Pause between API calls")
		retries        = flag.Int("retries", 3, "Retries per failed API call")
		skipDetails    = flag.Bool("no-items", false, "Skip per-receipt line item lookups (faster, less detail)")
		force          = flag.Bool("force", false, "Re-download receipts that were already saved")
		quiet          = flag.Bool("quiet", false, "Print only the final summary")
		browserLogin   = flag.Bool("browser-login", false, "Sign in through a browser window the tool opens, instead of the console command")
		browserPath    = flag.String("browser", "", "Browser executable for -browser-login (default: auto-detect Chrome, Edge, Chromium or Brave)")
		nonInteractive = flag.Bool("non-interactive", false, "Never ask for a sign-in; fail instead (for unattended runs)")
	)

	flag.Usage = usage
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	signInSettings := signInConfig{
		BrowserLogin:   *browserLogin,
		BrowserPath:    *browserPath,
		NonInteractive: *nonInteractive,
	}

	switch *command {
	case "login":
		exitOnError(signIn(ctx, signInSettings, os.Stdout))
	case "snippet":
		fmt.Println(costco.ConsoleSnippet())
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
			SignIn:      signInSettings,
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

The first time, it asks you to sign in: sign in to costco.com in your usual
browser, open Orders & Returns, and run the command it shows in the browser's
console (F12). The console copies your sign-in; press Enter back here and the
download starts. The sign-in is reused for about 90 days.

More:
  costco-cli -out ~/costco       Download into a different directory
  costco-cli -since 2020-01-01   Limit how far back to reach
  costco-cli -no-items           Skip receipt line items for a quick pass
  costco-cli -cmd login          Sign in again without downloading
  costco-cli -cmd snippet        Print the console command on its own
  costco-cli -browser-login      Sign in through a window the tool opens instead
  costco-cli -cmd info           Show where config and tokens are stored
  costco-cli -cmd setup          Set a warehouse number other than the default

Downloads resume: re-running skips receipts already on disk, so an interrupted
run only fetches what is missing. Use -force to re-download everything.

Flags:
`)
	flag.PrintDefaults()
}
