package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/0xHackerSpace/gh-xeb/internal/gh"
	"github.com/spf13/cobra"
)

// readOrgScope is the OAuth scope Vault's GitHub auth method requires in order
// to read the org and team memberships it maps to policies.
const readOrgScope = "read:org"

func newDoctorCmd(deps Deps) *cobra.Command {
	var asJSON bool

	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check this environment's GitHub identity and Vault readiness",
		Long: `doctor reports the GitHub identity attributes that HashiCorp Vault's GitHub
auth method consumes: the authenticated user, the token's scopes, and the
organisation and team memberships Vault maps to policies.

It is read-only, and it never contacts a Vault server. Only VAULT_ADDR is
inspected, and only from the environment.

With --json it emits those attributes as a structured document for other
tooling to consume. Filter it with jq:

  gh xeb doctor --json | jq -r '.identity.teams[] | .org + "/" + .slug'

Exits non-zero if any check fails, in both output modes. Warnings do not
affect the exit code.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			f := gather(deps)
			sections := checksFrom(f)

			if asJSON {
				if err := writeJSON(c.OutOrStdout(), f, sections); err != nil {
					return err
				}
			} else {
				report(c.OutOrStdout(), sections)
			}

			if count(sections, statusFail) > 0 {
				// The report already explains what is wrong; main only needs
				// to set the exit code.
				return SilentError{Err: errors.New("doctor found failing checks")}
			}
			return nil
		},
	}

	c.Flags().BoolVar(&asJSON, "json", false, "Emit the identity attributes and check results as JSON")

	return c
}

// ---------------------------------------------------------------------------
// Gathering
// ---------------------------------------------------------------------------

// team is one GitHub team membership, in the shape Vault maps to a policy.
type team struct {
	Org  string `json:"org"`
	Slug string `json:"slug"`
}

// facts is everything doctor learned about the environment. Gathering is
// separated from rendering so the human report and the JSON payload are
// guaranteed to describe the same run.
type facts struct {
	ghVersion    string
	ghVersionErr error

	auth        gh.Auth
	user        user
	scopes      []string
	identityErr error

	orgs     []string
	orgsErr  error
	teams    []team
	teamsErr error

	// clientErr is set when the REST client could not be built at all, in
	// which case every API-backed fact is unknown rather than failed.
	clientErr error

	vaultAddr string
}

func gather(deps Deps) facts {
	f := facts{
		scopes:    []string{},
		orgs:      []string{},
		teams:     []team{},
		auth:      deps.CurrentAuth(),
		vaultAddr: deps.Getenv("VAULT_ADDR"),
	}
	f.ghVersion, f.ghVersionErr = deps.CLIVersion()

	client, err := deps.NewRESTClient()
	if err != nil {
		f.clientErr = err
		return f
	}

	f.user, f.scopes, f.identityErr = fetchIdentity(client)
	f.orgs, f.orgsErr = fetchOrgs(client)
	f.teams, f.teamsErr = fetchTeams(client)
	return f
}

// hasReadOrg reports whether the token carries the scope Vault needs.
func (f facts) hasReadOrg() bool {
	for _, s := range f.scopes {
		if s == readOrgScope {
			return true
		}
	}
	return false
}

// fetchIdentity reads the user and the token's scopes from a single request —
// the scopes are only available as a response header.
func fetchIdentity(client gh.RESTClient) (user, []string, error) {
	resp, err := client.Request(http.MethodGet, "user", nil)
	if err != nil {
		return user{}, []string{}, err
	}
	defer resp.Body.Close()

	var u user
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return user{}, []string{}, fmt.Errorf("decoding user: %w", err)
	}
	return u, parseScopes(resp.Header.Get("X-OAuth-Scopes")), nil
}

func fetchOrgs(client gh.RESTClient) ([]string, error) {
	var payload []struct {
		Login string `json:"login"`
	}
	if err := client.Get("user/orgs", &payload); err != nil {
		return []string{}, err
	}

	orgs := make([]string, 0, len(payload))
	for _, o := range payload {
		orgs = append(orgs, o.Login)
	}
	return orgs, nil
}

func fetchTeams(client gh.RESTClient) ([]team, error) {
	var payload []struct {
		Slug         string `json:"slug"`
		Organization struct {
			Login string `json:"login"`
		} `json:"organization"`
	}
	if err := client.Get("user/teams", &payload); err != nil {
		return []team{}, err
	}

	teams := make([]team, 0, len(payload))
	for _, t := range payload {
		teams = append(teams, team{Org: t.Organization.Login, Slug: t.Slug})
	}
	return teams, nil
}

func parseScopes(header string) []string {
	scopes := []string{}
	for _, s := range strings.Split(header, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

// ---------------------------------------------------------------------------
// Checks
// ---------------------------------------------------------------------------

type status int

const (
	statusOK status = iota
	statusWarn
	statusFail
)

func (s status) String() string {
	switch s {
	case statusOK:
		return "ok"
	case statusWarn:
		return "warn"
	default:
		return "fail"
	}
}

// check is one line of the report: a name, what we found, and an optional
// follow-up telling the user what to do about it.
type check struct {
	status status
	name   string
	detail string
	note   string
}

type section struct {
	title  string
	checks []check
}

func ok(name, detail, note string) check {
	return check{status: statusOK, name: name, detail: detail, note: note}
}

func warn(name, detail, note string) check {
	return check{status: statusWarn, name: name, detail: detail, note: note}
}

func failed(name string, err error) check {
	return check{status: statusFail, name: name, detail: err.Error()}
}

// unavailable marks a check that could not run because an earlier one failed.
// Repeating the same underlying error on every dependent line buries the one
// place the user actually has to fix.
func unavailable(name string) check {
	return check{status: statusFail, name: name, detail: "not checked (no API access)"}
}

func checksFrom(f facts) []section {
	github := []check{cliVersionCheck(f), authCheck(f)}
	vault := []check{}

	switch {
	case f.clientErr != nil:
		github = append(github, failed("identity", f.clientErr), unavailable("token scopes"))
		vault = append(vault,
			unavailable(readOrgScope+" scope"),
			unavailable("org membership"),
			unavailable("team membership"),
		)

	case f.identityErr != nil:
		github = append(github, failed("identity", f.identityErr), unavailable("token scopes"))
		vault = append(vault, unavailable(readOrgScope+" scope"), orgCheck(f), teamCheck(f))

	default:
		github = append(github,
			ok("identity", fmt.Sprintf("@%s (id %d)", f.user.Login, f.user.ID), ""),
			scopesCheck(f),
		)
		vault = append(vault, readOrgCheck(f), orgCheck(f), teamCheck(f))
	}

	vault = append(vault, vaultAddrCheck(f))

	return []section{
		{title: "GitHub", checks: github},
		{title: "Vault readiness", checks: vault},
	}
}

func cliVersionCheck(f facts) check {
	if f.ghVersionErr != nil {
		return warn("gh CLI", "not found on PATH", "install it from https://cli.github.com")
	}
	return ok("gh CLI", f.ghVersion, "")
}

func authCheck(f facts) check {
	if !f.auth.HasToken {
		return check{
			status: statusFail,
			name:   "authentication",
			detail: "no token for " + f.auth.Host,
			note:   "run: gh auth login",
		}
	}
	return ok("authentication", fmt.Sprintf("%s (%s)", f.auth.Host, f.auth.Source), "")
}

func scopesCheck(f facts) check {
	if len(f.scopes) == 0 {
		return warn("token scopes", "none reported",
			"fine-grained tokens report no classic scopes")
	}
	return ok("token scopes", strings.Join(f.scopes, ", "), "")
}

func readOrgCheck(f facts) check {
	name := readOrgScope + " scope"
	if f.hasReadOrg() {
		return ok(name, "present", "required by Vault's GitHub auth method")
	}
	return warn(name, "missing", "run: gh auth refresh -s "+readOrgScope)
}

func orgCheck(f facts) check {
	if f.orgsErr != nil {
		return failed("org membership", f.orgsErr)
	}
	if len(f.orgs) == 0 {
		return warn("org membership", "none visible",
			"Vault matches policies on org membership; check the "+readOrgScope+" scope")
	}
	return ok("org membership", strings.Join(f.orgs, ", "), "")
}

func teamCheck(f facts) check {
	if f.teamsErr != nil {
		return failed("team membership", f.teamsErr)
	}
	if len(f.teams) == 0 {
		return warn("team membership", "none visible",
			"Vault maps teams to policies; without one only org-wide policies apply")
	}

	names := make([]string, 0, len(f.teams))
	for _, t := range f.teams {
		names = append(names, t.Org+"/"+t.Slug)
	}
	return ok("team membership", strings.Join(names, ", "), "")
}

func vaultAddrCheck(f facts) check {
	if f.vaultAddr != "" {
		return ok("VAULT_ADDR", f.vaultAddr, "")
	}
	return warn("VAULT_ADDR", "not set", "export VAULT_ADDR=https://vault.example.com:8200")
}

func count(sections []section, s status) int {
	n := 0
	for _, sec := range sections {
		for _, c := range sec.checks {
			if c.status == s {
				n++
			}
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// report renders the sections with the name column aligned across every
// section, so the whole report reads as one table.
func report(w io.Writer, sections []section) {
	width := 0
	for _, sec := range sections {
		for _, c := range sec.checks {
			if len(c.name) > width {
				width = len(c.name)
			}
		}
	}

	for _, sec := range sections {
		fmt.Fprintln(w, sec.title)
		for _, c := range sec.checks {
			line := fmt.Sprintf("  %-4s %-*s %s", c.status, width+2, c.name, c.detail)
			fmt.Fprintln(w, strings.TrimRight(line, " "))
			if c.note != "" {
				fmt.Fprintf(w, "       (%s)\n", c.note)
			}
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, summary(sections))
}

func summary(sections []section) string {
	parts := []string{fmt.Sprintf("%d passed", count(sections, statusOK))}
	if n := count(sections, statusWarn); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", n, plural(n, "warning")))
	}
	if n := count(sections, statusFail); n > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", n))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// ---------------------------------------------------------------------------
// JSON output
//
// This payload is a public interface: other tooling consumes it. Fields may be
// added, but renaming or removing one is a breaking change.
// See docs/adr/0012-doctor-json-output.md.
// ---------------------------------------------------------------------------

type jsonPayload struct {
	GHVersion string       `json:"ghVersion"`
	Identity  jsonIdentity `json:"identity"`
	Vault     jsonVault    `json:"vault"`
	Checks    []jsonCheck  `json:"checks"`
	Summary   jsonSummary  `json:"summary"`
}

// jsonIdentity is the point of the whole command: the GitHub attributes
// Vault's GitHub auth method consumes, in a shape a program can read.
type jsonIdentity struct {
	Host          string   `json:"host"`
	Authenticated bool     `json:"authenticated"`
	TokenSource   string   `json:"tokenSource"`
	Login         string   `json:"login"`
	ID            int64    `json:"id"`
	Scopes        []string `json:"scopes"`
	Orgs          []string `json:"orgs"`
	Teams         []team   `json:"teams"`
}

type jsonVault struct {
	Addr string `json:"addr"`
	// ReadOrgScope is broken out because it is the single most common reason a
	// Vault GitHub login fails.
	ReadOrgScope bool `json:"readOrgScope"`
}

type jsonCheck struct {
	Section string `json:"section"`
	Status  string `json:"status"`
	Name    string `json:"name"`
	Detail  string `json:"detail"`
	Note    string `json:"note,omitempty"`
}

type jsonSummary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
}

func writeJSON(w io.Writer, f facts, sections []section) error {
	checks := []jsonCheck{}
	for _, sec := range sections {
		for _, c := range sec.checks {
			checks = append(checks, jsonCheck{
				Section: sec.title,
				Status:  c.status.String(),
				Name:    c.name,
				Detail:  c.detail,
				Note:    c.note,
			})
		}
	}

	payload := jsonPayload{
		GHVersion: f.ghVersion,
		Identity: jsonIdentity{
			Host:          f.auth.Host,
			Authenticated: f.auth.HasToken,
			TokenSource:   f.auth.Source,
			Login:         f.user.Login,
			ID:            f.user.ID,
			Scopes:        f.scopes,
			Orgs:          f.orgs,
			Teams:         f.teams,
		},
		Vault: jsonVault{
			Addr:         f.vaultAddr,
			ReadOrgScope: f.hasReadOrg(),
		},
		Checks: checks,
		Summary: jsonSummary{
			Passed:   count(sections, statusOK),
			Warnings: count(sections, statusWarn),
			Failed:   count(sections, statusFail),
		},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encoding JSON report: %w", err)
	}
	return nil
}
