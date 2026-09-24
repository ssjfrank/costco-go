package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

// consoleSignIn walks the user through signing in with the console snippet:
// they sign in to costco.com in their own browser, run the snippet in DevTools,
// and the sign-in it copies is picked up from the clipboard or pasted here.
type consoleSignIn struct {
	lines     <-chan string
	out       io.Writer
	clipboard func(context.Context) (string, error)

	// stale is a refresh token Costco has just refused. A clipboard still
	// holding it is ignored instead of being imported again.
	stale string
}

func (s consoleSignIn) run(ctx context.Context) (*costco.TokenResponse, error) {
	if tokens := s.fromClipboard(ctx); tokens != nil {
		return tokens, nil
	}
	s.printInstructions()

	var pasted strings.Builder
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case line, ok := <-s.lines:
			if !ok {
				return nil, errors.New("no sign-in was pasted")
			}

			if pasted.Len() == 0 && strings.TrimSpace(line) == "" {
				if tokens := s.fromClipboard(ctx); tokens != nil {
					return tokens, nil
				}
				fmt.Fprintln(s.out, "No Costco sign-in in the clipboard yet. Run the command in the Costco console, then press Enter again.")
				continue
			}

			pasted.WriteString(line)
			pasted.WriteString("\n")
			tokens, complete, err := parsePasted(pasted.String())
			if !complete {
				continue
			}
			pasted.Reset()
			if err != nil {
				fmt.Fprintf(s.out, "That did not work: %v\nRun the console command again, then press Enter.\n", err)
				continue
			}
			return tokens, nil
		}
	}
}

// parsePasted parses what has been pasted so far. complete is false while the
// paste is still arriving: a sign-in block without its END line yet, or JSON
// that has not been closed.
func parsePasted(text string) (tokens *costco.TokenResponse, complete bool, err error) {
	tokens, err = costco.ParseSignIn(text)
	if err == nil {
		return tokens, true, nil
	}
	if strings.Contains(text, "BEGIN COSTCO SIGN-IN") && !strings.Contains(text, "END COSTCO SIGN-IN") {
		return nil, false, nil
	}
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		return nil, false, nil
	}
	return nil, true, err
}

func (s consoleSignIn) fromClipboard(ctx context.Context) *costco.TokenResponse {
	if s.clipboard == nil {
		return nil
	}
	text, err := s.clipboard(ctx)
	if err != nil {
		return nil
	}
	tokens, err := costco.ParseSignIn(text)
	if err != nil || tokens.RefreshToken == s.stale {
		return nil
	}
	fmt.Fprintln(s.out, "Found a Costco sign-in in your clipboard.")
	return tokens
}

func (s consoleSignIn) printInstructions() {
	fmt.Fprintf(s.out, `To sign in:

  1. In your usual browser, sign in at https://www.costco.com and open Orders & Returns.
  2. Press F12 (Mac: Cmd+Option+J) to open the Console, paste the command below and press Enter.
     If the browser asks, type "allow pasting" and press Enter first.

%s

  3. The console copies your sign-in. Come back here and press Enter
     (or paste what the console printed). Press Ctrl-C to cancel.

`, costco.ConsoleSnippet())
}

// readLines delivers r line by line. One reader is shared by every prompt in a
// run, so a line typed at one prompt can never be swallowed by an earlier one.
func readLines(r io.Reader) <-chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return lines
}
