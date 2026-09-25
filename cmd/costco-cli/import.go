package main

import (
	"fmt"
	"io"
	"os"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

// importTokens reads a whole sign-in from in: the block printed by the console
// snippet, or the raw JSON response of Costco's token endpoint. It suits
// redirected input ('costco-cli -cmd import-token < token.json'); interactive
// sign-in goes through consoleSignIn instead.
func importTokens(in io.Reader, out io.Writer) error {
	fmt.Fprintln(out, "Paste the sign-in printed by the Costco console command (or a token response JSON), then press Ctrl+D:")
	fmt.Fprintln(out)

	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	response, err := costco.ParseSignIn(string(data))
	if err != nil {
		return err
	}

	tokens, err := costco.ImportTokenResponse(response)
	if err != nil {
		return err
	}

	if err = costco.SaveTokens(tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	path, _ := costco.TokenFilePath()
	fmt.Fprintf(out, "✓ Tokens saved to %s\n", path)
	fmt.Fprintf(out, "  ID token valid until:      %s\n", tokens.TokenExpiry.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(out, "  Refresh token valid until: %s\n", tokens.RefreshTokenExpiresAt.Format("2006-01-02 15:04:05 MST"))
	return nil
}

func runImportTokens() error {
	return importTokens(os.Stdin, os.Stdout)
}
