package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/0xHackerSpace/gh-cli-extension/internal/vault"
	"github.com/spf13/cobra"
)

// displayedCapabilities is the set shown by `vault can`, in the order Vault's
// own documentation lists them. Vault may return others (sudo, root, deny);
// those are reported separately rather than silently dropped.
var displayedCapabilities = []string{"create", "read", "update", "patch", "delete", "list"}

func newVaultCmd(deps Deps) *cobra.Command {
	var opts vaultOpts

	root := &cobra.Command{
		Use:   "vault",
		Short: "Show the Vault resources this token can reach",
		Long: `vault reports what your Vault token actually gives you: the server it is
talking to, the token's own policies and lifetime, and the secret engines
visible to you along with what you may do at each.

Configuration comes from the standard VAULT_* environment variables, so
VAULT_ADDR, VAULT_NAMESPACE and the TLS settings behave exactly as they do for
the vault CLI.

The Vault token is resolved in the same order the vault CLI uses:

  1. VAULT_TOKEN
  2. ~/.vault-token

If neither exists, this logs in through Vault's GitHub auth method using your
gh credentials. That token lives only for the invocation and is never written
to disk, so nothing is left behind on the machine -- at the cost of a fresh
login, and a new accessor in Vault, on every run. Run 'vault login' yourself if
you would rather reuse one token.

Every subcommand accepts --json.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return withClient(c, deps, opts, func(ctx context.Context, client vault.Client) error {
				return runOverview(c.OutOrStdout(), ctx, client, opts.asJSON)
			})
		},
	}

	root.PersistentFlags().BoolVar(&opts.asJSON, "json", false, "Emit the result as JSON")
	root.PersistentFlags().StringVar(&opts.authPath, "auth-path", "github",
		"Mount path of Vault's GitHub auth method, used only when no Vault token exists")

	root.AddCommand(
		newVaultTokenCmd(deps, &opts),
		newVaultMountsCmd(deps, &opts),
		newVaultCanCmd(deps, &opts),
		newVaultGetCmd(deps, &opts),
	)

	return root
}

// vaultOpts carries the flags shared by every vault subcommand.
type vaultOpts struct {
	asJSON   bool
	authPath string
}

// withClient builds an authenticated client and hands it to fn, turning the
// "no token anywhere" case into advice rather than a bare error.
func withClient(c *cobra.Command, deps Deps, opts vaultOpts, fn func(context.Context, vault.Client) error) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	client, err := deps.NewVaultClient(ctx, vault.Options{
		GitHubToken: deps.GitHubToken,
		AuthPath:    opts.authPath,
	})
	if err != nil {
		if errors.Is(err, vault.ErrNoToken) {
			return fmt.Errorf("%w\n\nSet VAULT_TOKEN, run `vault login`, or authenticate with\n"+
				"GitHub by running `gh auth login` so this command can log in for you", err)
		}
		return err
	}

	return fn(ctx, client)
}

// ---------------------------------------------------------------------------
// vault (overview)
// ---------------------------------------------------------------------------

func runOverview(w io.Writer, ctx context.Context, client vault.Client, asJSON bool) error {
	health, err := client.Health(ctx)
	if err != nil {
		return err
	}
	token, err := client.Token(ctx)
	if err != nil {
		return err
	}
	mounts, err := client.Mounts(ctx)
	if err != nil {
		return err
	}

	if asJSON {
		return encodeJSON(w, map[string]interface{}{
			"address": client.Address(),
			"health":  health,
			"token":   token,
			"mounts":  mounts,
		})
	}

	fmt.Fprintf(w, "Vault  %s\n", client.Address())
	fmt.Fprintf(w, "       %s\n\n", describeHealth(health))

	fmt.Fprintln(w, "Token")
	writeTokenLines(w, token, "  ")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Mounts (%d visible)\n", len(mounts))
	writeMountLines(w, mounts, "  ")
	return nil
}

func describeHealth(h vault.Health) string {
	state := "unsealed"
	switch {
	case h.Sealed:
		state = "SEALED"
	case !h.Initialized:
		state = "not initialised"
	case h.Standby:
		state = "standby"
	}

	version := h.Version
	if version == "" {
		version = "unknown version"
	} else {
		version = "v" + strings.TrimPrefix(version, "v")
	}

	if h.ClusterName != "" {
		return fmt.Sprintf("%s, %s (%s)", version, state, h.ClusterName)
	}
	return fmt.Sprintf("%s, %s", version, state)
}

// ---------------------------------------------------------------------------
// vault token
// ---------------------------------------------------------------------------

func newVaultTokenCmd(deps Deps, opts *vaultOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Show the current Vault token's policies and lifetime",
		Long: `token reports what Vault says about the token you are using: its display
name, the policies attached to it, how long it has left, and the identity
entity it resolves to.

The token value itself is never printed.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return withClient(c, deps, *opts, func(ctx context.Context, client vault.Client) error {
				token, err := client.Token(ctx)
				if err != nil {
					return err
				}
				if opts.asJSON {
					return encodeJSON(c.OutOrStdout(), token)
				}
				writeTokenLines(c.OutOrStdout(), token, "")
				return nil
			})
		},
	}
}

func writeTokenLines(w io.Writer, t vault.Token, indent string) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)

	name := t.DisplayName
	if name == "" {
		name = "(none)"
	}
	fmt.Fprintf(tw, "%sdisplay name\t%s\n", indent, name)

	policies := "(none)"
	if len(t.Policies) > 0 {
		policies = strings.Join(t.Policies, ", ")
	}
	fmt.Fprintf(tw, "%spolicies\t%s\n", indent, policies)

	ttl := vault.FormatTTL(t.TTL)
	if t.Renewable {
		ttl += " (renewable)"
	}
	fmt.Fprintf(tw, "%sttl\t%s\n", indent, ttl)

	if t.EntityID != "" {
		fmt.Fprintf(tw, "%sentity\t%s\n", indent, t.EntityID)
	}
	if t.AuthPath != "" {
		fmt.Fprintf(tw, "%ssource\t%s (logged in for this run)\n", indent, t.AuthPath)
	}

	tw.Flush()
}

// ---------------------------------------------------------------------------
// vault mounts
// ---------------------------------------------------------------------------

func newVaultMountsCmd(deps Deps, opts *vaultOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "mounts",
		Short: "List the secret engines visible to this token",
		Long: `mounts lists the secret engines your token can see, with the capabilities you
hold at each mount path.

Those capabilities are for the mount path itself. For a KV v2 engine the actual
secrets live under <mount>data/<path>, which can carry different capabilities --
use 'vault can' to check a specific path.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return withClient(c, deps, *opts, func(ctx context.Context, client vault.Client) error {
				mounts, err := client.Mounts(ctx)
				if err != nil {
					return err
				}
				if opts.asJSON {
					return encodeJSON(c.OutOrStdout(), map[string]interface{}{"mounts": mounts})
				}
				writeMountLines(c.OutOrStdout(), mounts, "")
				return nil
			})
		},
	}
}

func writeMountLines(w io.Writer, mounts []vault.Mount, indent string) {
	if len(mounts) == 0 {
		fmt.Fprintf(w, "%s(none visible to this token)\n", indent)
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, m := range mounts {
		caps := strings.Join(m.Capabilities, ", ")
		if caps == "" {
			caps = "-"
		}
		fmt.Fprintf(tw, "%s%s\t%s\t%s\n", indent, m.Path, m.Type, caps)
	}
	tw.Flush()
}

// ---------------------------------------------------------------------------
// vault can
// ---------------------------------------------------------------------------

func newVaultCanCmd(deps Deps, opts *vaultOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "can <path>",
		Short: "Show what this token may do at a Vault path",
		Long: `can resolves your token's capabilities at one path, the same way Vault does
when it authorises a request.

The path is the API path, so a KV v2 secret at 'prod/db' under the mount 'kv/'
is 'kv/data/prod/db'.`,
		Example: "  gh cli-extension vault can kv/data/prod/db",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := args[0]
			return withClient(c, deps, *opts, func(ctx context.Context, client vault.Client) error {
				caps, err := client.Capabilities(ctx, path)
				if err != nil {
					return err
				}
				if opts.asJSON {
					return encodeJSON(c.OutOrStdout(), capabilityPayload(path, caps))
				}
				writeCapabilities(c.OutOrStdout(), path, caps)
				return nil
			})
		},
	}
}

// allows reports whether the capability set permits an action. Vault's "root"
// permits everything and "deny" overrides everything else.
func allows(caps []string, action string) bool {
	denied, root, found := false, false, false
	for _, c := range caps {
		switch c {
		case "deny":
			denied = true
		case "root":
			root = true
		case action:
			found = true
		}
	}
	if denied {
		return false
	}
	return root || found
}

func capabilityPayload(path string, caps []string) map[string]interface{} {
	allowed := map[string]bool{}
	for _, action := range displayedCapabilities {
		allowed[action] = allows(caps, action)
	}
	if caps == nil {
		caps = []string{}
	}
	return map[string]interface{}{
		"path":         path,
		"capabilities": caps,
		"allowed":      allowed,
	}
}

func writeCapabilities(w io.Writer, path string, caps []string) {
	fmt.Fprintln(w, path)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, action := range displayedCapabilities {
		answer := "no"
		if allows(caps, action) {
			answer = "yes"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", action, answer)
	}
	tw.Flush()

	// Anything Vault returned that is not in the table above still matters.
	var extra []string
	for _, c := range caps {
		switch c {
		case "sudo", "root", "deny":
			extra = append(extra, c)
		}
	}
	if len(extra) > 0 {
		fmt.Fprintf(w, "\n  also: %s\n", strings.Join(extra, ", "))
	}
}

// ---------------------------------------------------------------------------
// vault get
// ---------------------------------------------------------------------------

func newVaultGetCmd(deps Deps, opts *vaultOpts) *cobra.Command {
	var reveal bool
	var field string

	c := &cobra.Command{
		Use:   "get <path>",
		Short: "Read a KV secret, with values masked by default",
		Long: `get reads a KV secret and, by default, shows which fields exist and how long
each value is -- without printing the values themselves. A secret should not
land in your scrollback, or in a CI log, just because you looked at it.

  --reveal        print every value
  --field <key>   print one value raw, with no formatting, for piping

--field implies you asked for that specific value, so it is never masked.

The path is the logical one the vault CLI takes ('secret/prod/db'), not the API
path. Whether Vault wants 'secret/data/prod/db' depends on the engine being KV
v2, which is resolved for you. An API path with the data/ segment already in it
is accepted too.

With --json the values are masked unless --reveal is given, and the payload
says which it was.`,
		Example: `  gh cli-extension vault get secret/prod/db
  gh cli-extension vault get secret/prod/db --reveal
  gh cli-extension vault get secret/prod/db --field password | pbcopy`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if field != "" && reveal {
				return errors.New("--field already prints the value; --reveal adds nothing")
			}

			return withClient(c, deps, *opts, func(ctx context.Context, client vault.Client) error {
				secret, err := client.ReadSecret(ctx, args[0])
				if err != nil {
					return err
				}

				if field != "" {
					value, ok := secret.Data[field]
					if !ok {
						return fmt.Errorf("no field %q in %s (has: %s)",
							field, secret.Path, strings.Join(secret.Keys(), ", "))
					}
					// Raw, unadorned, newline-terminated: this is piped.
					fmt.Fprintln(c.OutOrStdout(), value)
					return nil
				}

				if opts.asJSON {
					return encodeJSON(c.OutOrStdout(), secretPayload(secret, reveal))
				}
				writeSecret(c.OutOrStdout(), secret, reveal)
				return nil
			})
		},
	}

	c.Flags().BoolVar(&reveal, "reveal", false, "Print the secret values instead of masking them")
	c.Flags().StringVar(&field, "field", "", "Print only this field's value, raw and unmasked")

	return c
}

func secretPayload(s vault.Secret, reveal bool) map[string]interface{} {
	data := make(map[string]string, len(s.Data))
	for k, v := range s.Data {
		if reveal {
			data[k] = v
		} else {
			data[k] = maskOf(v)
		}
	}
	return map[string]interface{}{
		"path":      s.Path,
		"mount":     s.Mount,
		"mountType": s.MountType,
		"version":   s.Version,
		"masked":    !reveal,
		"data":      data,
	}
}

func writeSecret(w io.Writer, s vault.Secret, reveal bool) {
	header := s.MountType
	if s.Version > 0 {
		header = fmt.Sprintf("%s, version %d", s.MountType, s.Version)
	}
	fmt.Fprintf(w, "%s  %s\n\n", s.Path, header)

	if len(s.Data) == 0 {
		fmt.Fprintln(w, "  (no fields)")
		return
	}

	// Buffered so trailing padding can be stripped: an empty field would
	// otherwise leave the key column's spaces dangling at end of line.
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)
	for _, k := range s.Keys() {
		value := s.Data[k]
		if !reveal {
			value = maskOf(value)
		}
		fmt.Fprintf(tw, "  %s\t%s\n", k, value)
	}
	tw.Flush()
	writeTrimmed(w, buf.String())

	if !reveal {
		fmt.Fprintln(w, "\n  --reveal to print values, --field <key> for one")
	}
}

// writeTrimmed emits tabwriter output with trailing padding removed from each
// line, so an empty value does not leave invisible whitespace behind.
func writeTrimmed(w io.Writer, block string) {
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

// maskOf hides a value but keeps its length, which is enough to tell an empty
// field from a populated one and a passphrase from a fingerprint.
func maskOf(value string) string {
	if value == "" {
		return "(empty)"
	}
	unit := "chars"
	if len(value) == 1 {
		unit = "char"
	}
	return fmt.Sprintf("******** (%d %s)", len(value), unit)
}
