package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"

	"github.com/spf13/cobra"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
)

var authExitCodes = map[string]string{"exitCodes": "0 success; 1 error (login failed, network error, or not authenticated)"}

// NewAuthCmd builds the `mr auth` command group.
func NewAuthCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "auth",
		Short:       "Log in, log out, and inspect the current identity",
		Long:        "Manage CLI authentication against a mahresources server. Logging in mints an API token and stores it so subsequent commands are authenticated automatically.",
		Annotations: authExitCodes,
	}
	cmd.AddCommand(newAuthLoginCmd(c, opts))
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthWhoamiCmd(c, opts))
	return cmd
}

func newAuthLoginCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var username, password, tokenName string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and store an API token",
		Long:  "Authenticate with a username and password, mint a personal API token, and store it in the credentials file. Subsequent mr commands read that token automatically; override it any time with the MR_TOKEN environment variable.",
		Example: strings.Join([]string{
			"  # Log in to the default server",
			"  mr auth login --username alice --password s3cret",
			"",
			"  # Log in to a specific server and name the token",
			"  mr --server https://mr.example.com auth login --username alice --password s3cret --name laptop",
		}, "\n"),
		Annotations: authExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			if username == "" || password == "" {
				return fmt.Errorf("--username and --password are required")
			}
			token, err := loginAndMintToken(c.BaseURL, username, password, tokenName)
			if err != nil {
				return err
			}
			if err := client.StoreToken(token); err != nil {
				return err
			}
			fmt.Println("Logged in. API token stored.")
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Account username")
	cmd.Flags().StringVar(&password, "password", "", "Account password")
	cmd.Flags().StringVar(&tokenName, "name", "mr cli", "Label for the minted API token")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored API token",
		Long:  "Delete the locally stored API token so this machine is no longer authenticated. This does not revoke the token on the server; use `mr token revoke` to invalidate it everywhere.",
		Example: strings.Join([]string{
			"  # Forget the stored credentials on this machine",
			"  mr auth logout",
			"",
			"  # Logout is safe to run even when not logged in",
			"  mr auth logout",
		}, "\n"),
		Annotations: authExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.ClearToken(); err != nil {
				return err
			}
			fmt.Println("Logged out (local token cleared).")
			return nil
		},
	}
}

func newAuthWhoamiCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated principal",
		Long:  "Print the identity and capabilities the server associates with the current credentials. Useful to confirm a token works and which role it has.",
		Example: strings.Join([]string{
			"  # Show the current identity",
			"  mr auth whoami",
			"",
			"  # As raw JSON",
			"  mr auth whoami --json",
		}, "\n"),
		Annotations: authExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			var me map[string]any
			if err := c.Get("/v1/auth/me", nil, &me); err != nil {
				return err
			}
			raw, _ := json.Marshal(me)
			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "Username", Value: fmt.Sprintf("%v", me["username"])},
				{Key: "Role", Value: fmt.Sprintf("%v", me["role"])},
				{Key: "IsAdmin", Value: fmt.Sprintf("%v", me["isAdmin"])},
				{Key: "CanWrite", Value: fmt.Sprintf("%v", me["canWrite"])},
				{Key: "ScopeGroupId", Value: fmt.Sprintf("%v", me["scopeGroupId"])},
			}, raw)
			return nil
		},
	}
}

// loginAndMintToken performs a cookie-based login then mints an API token using
// that session. It is self-contained (its own cookie jar) so it works before any
// token is stored.
func loginAndMintToken(baseURL, username, password, name string) (string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	hc := &http.Client{Jar: jar}
	base := strings.TrimRight(baseURL, "/")

	loginBody, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := hc.Post(base+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: HTTP %d", resp.StatusCode)
	}

	// Minting a token is a state-changing, cookie-authenticated request, so when
	// the server has auth enabled it requires the session's CSRF token. Read it
	// from /v1/auth/me (which echoes csrfToken for the current session) and send
	// it as the X-CSRF-Token header. With auth disabled the token is empty and
	// the header is simply ignored.
	csrfToken, err := fetchCSRFToken(hc, base)
	if err != nil {
		return "", err
	}

	if name == "" {
		name = "mr cli"
	}
	mintBody, _ := json.Marshal(map[string]string{"name": name})
	mintReq, err := http.NewRequest(http.MethodPost, base+"/v1/account/tokens", bytes.NewReader(mintBody))
	if err != nil {
		return "", err
	}
	mintReq.Header.Set("Content-Type", "application/json")
	if csrfToken != "" {
		mintReq.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp2, err := hc.Do(mintReq)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not mint API token: HTTP %d", resp2.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("server did not return a token")
	}
	return out.Token, nil
}

// fetchCSRFToken reads the current session's CSRF token from /v1/auth/me using
// the (cookie-bearing) http client. Returns "" when the server has auth disabled
// (no token is issued), which is not an error.
func fetchCSRFToken(hc *http.Client, base string) (string, error) {
	resp, err := hc.Get(base + "/v1/auth/me")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not read session info: HTTP %d", resp.StatusCode)
	}
	var me struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return "", err
	}
	return me.CsrfToken, nil
}
