package costco

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	signInBlockBegin = "-----BEGIN COSTCO SIGN-IN-----"
	signInBlockEnd   = "-----END COSTCO SIGN-IN-----"

	// assumedRefreshLifetime stands in for refresh_token_expires_in when the
	// snippet could not ask Costco for fresh tokens. It is what Costco reports
	// for new refresh tokens, and the first refresh replaces it with the real value.
	assumedRefreshLifetime = 90 * 24 * 60 * 60
)

// consoleSnippetSource is run in the browser console on costco.com. It finds the
// sign-in that Costco's order history app (MSAL.js) keeps in the page's storage,
// asks Costco for fresh tokens with it, exactly as the app itself does, and
// copies the result as a block of short lines. Short lines matter: macOS
// terminals cut a pasted line off at 1024 characters, and a token is several KB.
const consoleSnippetSource = `
(async () => {
  const CLIENT = "__CLIENT__", TOKEN_URL = "__TOKEN_URL__";
  const toClipboard = typeof copy === "function" ? copy : null;
  const found = {};
  let others = 0;
  for (const store of [sessionStorage, localStorage]) {
    for (let i = 0; i < store.length; i++) {
      let entry;
      try { entry = JSON.parse(store.getItem(store.key(i))); } catch (e) { continue; }
      if (!entry || !entry.secret || !/^(IdToken|RefreshToken)$/.test(entry.credentialType)) continue;
      if (entry.clientId !== CLIENT) { others++; continue; }
      found[entry.credentialType] = entry.secret;
    }
  }
  if (!found.RefreshToken) {
    console.error(others ? "This page has a sign-in for a different Costco app. Open Orders & Returns and run this again." : "No Costco sign-in on this page. Sign in, open Orders & Returns, then run this again.");
    return;
  }
  let tokens, response;
  try {
    response = await fetch(TOKEN_URL, {method: "POST", body: new URLSearchParams({client_id: CLIENT, grant_type: "refresh_token", refresh_token: found.RefreshToken})});
    const text = await response.text();
    try { tokens = JSON.parse(text); } catch (e) { tokens = {}; }
  } catch (e) {
    if (!found.IdToken) { console.error("Could not reach Costco: " + e); return; }
    console.warn("Could not reach Costco to refresh the sign-in; using the one on this page.");
    tokens = {id_token: found.IdToken, refresh_token: found.RefreshToken, refresh_token_expires_in: __LIFETIME__};
  }
  if (response && (!response.ok || !tokens.refresh_token)) {
    console.error("Costco refused the sign-in (" + (tokens.error_description || tokens.error || response.status) + "). Sign out, sign in again and rerun this.");
    return;
  }
  const block = "__BEGIN__\n" + btoa(unescape(encodeURIComponent(JSON.stringify(tokens)))).match(/.{1,64}/g).join("\n") + "\n__END__";
  if (toClipboard) toClipboard(block);
  console.log(block + "\n\n" + (toClipboard ? "Copied to your clipboard. " : "Copy everything from BEGIN to END. ") + "Now run costco-cli.");
  return block;
})()
`

// ConsoleSnippet returns the one-line command to paste into the browser console
// on costco.com after signing in. It prints, and copies to the clipboard, a
// sign-in block that ParseSignIn reads.
//
// The only network request it makes is the token refresh the order history app
// makes itself, to Costco's own sign-in endpoint.
func ConsoleSnippet() string {
	return consoleSnippet(TokenEndpoint, ClientID)
}

func consoleSnippet(tokenURL, clientID string) string {
	var lines []string
	for _, line := range strings.Split(consoleSnippetSource, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.NewReplacer(
		"__CLIENT__", clientID,
		"__TOKEN_URL__", tokenURL,
		"__LIFETIME__", strconv.Itoa(assumedRefreshLifetime),
		"__BEGIN__", signInBlockBegin,
		"__END__", signInBlockEnd,
	).Replace(strings.Join(lines, " "))
}

// ParseSignIn reads a sign-in in either form users paste: the block printed by
// ConsoleSnippet, or the raw JSON response of Costco's token endpoint. Text
// around the block and any line breaks or indentation inside it are ignored, so
// copying from a console, terminal or editor all work.
func ParseSignIn(text string) (*TokenResponse, error) {
	begin := strings.Index(text, signInBlockBegin)
	if begin < 0 {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil, errors.New("nothing was pasted")
		}
		return decodeSignIn([]byte(trimmed))
	}

	rest := text[begin+len(signInBlockBegin):]
	end := strings.Index(rest, signInBlockEnd)
	if end < 0 {
		return nil, fmt.Errorf("the sign-in is incomplete: copy everything from %s to %s", signInBlockBegin, signInBlockEnd)
	}

	encoded := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, rest[:end])
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("the sign-in is damaged, copy it again: %w", err)
	}
	return decodeSignIn(decoded)
}

func decodeSignIn(data []byte) (*TokenResponse, error) {
	var response TokenResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w. Paste the block printed by the Costco console command", err)
	}
	if response.IDToken == "" {
		return nil, errors.New("id_token is missing from the sign-in")
	}
	if response.RefreshToken == "" {
		return nil, errors.New("refresh_token is missing from the sign-in")
	}
	return &response, nil
}
