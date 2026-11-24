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
		browserLogin   = flag.Bool("browser-login", false, "Sign in only through the browser window the tool opens; never fall back to the console command")
		consoleLogin   = flag.Bool("console-login", false, "Sign in only with the console command in your own browser; never open a window")
		tempProfile    = flag.Bool("temp-profile", false, "Give the sign-in window a throwaway profile, so no Costco cookies are kept on disk")
		browserPath    = flag.String("browser", "", "Browser executable for the sign-in window (default: auto-detect Chrome, Edge, Chromium or Brave)")
		nonInteractive = flag.Bool("non-interactive", false, "Never ask for a sign-in; fail instead (for unattended runs)")
		showVersion    = flag.Bool("version", false, "Print the version and exit")
	)

	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(versionString(commit))
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	signInSettings := signInConfig{
		BrowserLogin:   *browserLogin,
		BrowserPath:    *browserPath,
		ConsoleLogin:   *consoleLogin,
		TempProfile:    *tempProfile,
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

The first time, a browser window opens on Costco's sign-in page. Sign in there;
the window closes by itself and the download starts. The sign-in is reused for
about 90 days. The window keeps its Costco cookies on this computer only
(~/.costco/browser-profile), so signing in again later may need no typing.

Without Chrome, Edge, Chromium or Brave, or if the window does not work, it
shows a command to run in the console (F12) of your own browser instead.

More:
  costco-cli -out ~/costco       Download into a different directory
  costco-cli -since 2020-01-01   Limit how far back to reach
  costco-cli -no-items           Skip receipt line items for a quick pass
  costco-cli -cmd login          Sign in again without downloading
  costco-cli -console-login      Sign in with the console command, never a window
  costco-cli -temp-profile       Sign in with a window that keeps no cookies
  costco-cli -cmd snippet        Print the console command on its own
  costco-cli -cmd info           Show where config, tokens and cookies are stored
  costco-cli -cmd setup          Set a warehouse number other than the default

Downloads resume: re-running skips receipts already on disk with their line
items, so an interrupted run only fetches what is missing. Use -force to
re-download everything.

Flags:
`)
	flag.PrintDefaults()
}
