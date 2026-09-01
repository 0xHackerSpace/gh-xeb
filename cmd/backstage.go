package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/0xHackerSpace/gh-cli-extension/internal/backstage"
	"github.com/0xHackerSpace/gh-cli-extension/internal/vault"
	"github.com/spf13/cobra"
)

// projectSlugAnnotation is how Backstage's GitHub integrations record which
// repository an entity came from. It is the only reliable link from a git
// remote back to a catalog entry.
const projectSlugAnnotation = "github.com/project-slug"

// The fields --vault-secret looks for, in order. The first name is the one to
// use; the second exists because it is what people reach for anyway.
var (
	vaultURLFields   = []string{"url", "base_url"}
	vaultTokenFields = []string{"token", "api_token"}
)

func newBackstageCmd(deps Deps) *cobra.Command {
	var opts backstageOpts

	root := &cobra.Command{
		Use:   "backstage",
		Short: "Query a Backstage software catalog",
		Long: `backstage reads the Software Catalog of a Backstage instance: which entities
exist, who owns them, what they depend on, and which one describes the
repository you are standing in.

The address comes from --url, or from BACKSTAGE_BASE_URL (or BACKSTAGE_URL) so
it can be set once per shell. Give the app's address, not the API path:

  https://backstage.example.com

Authentication is a bearer token read from BACKSTAGE_TOKEN. There is
deliberately no --token flag: a token passed on the command line lands in your
shell history and in the output of ps, where anyone on the machine can read it.
Catalogs that allow anonymous reads work with no token at all.

--vault-secret takes both out of your environment entirely, reading them from a
Vault KV secret instead:

  gh cli-extension backstage --vault-secret secret/backstage

That secret should carry a 'url' field, a 'token' field, or both ('base_url'
and 'api_token' are accepted too). Vault is reached exactly as the vault
command reaches it -- VAULT_ADDR, VAULT_TOKEN, ~/.vault-token, and failing
those a login through Vault's GitHub auth method with your gh credentials, so
being logged in to gh can be enough to query a catalog you hold no local
credential for.

Where each value comes from, first match winning:

  url     --url, then the Vault secret, then BACKSTAGE_BASE_URL, BACKSTAGE_URL
  token   the Vault secret, then BACKSTAGE_TOKEN

The token is passed straight from Vault to the catalog request. It is never
printed, never logged, and never written to disk.

This command only reads. Entities are created by registering a catalog-info.yaml
in a repository, which is a git operation, not an API call.

Every subcommand accepts --json.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return withBackstage(c, deps, opts, func(ctx context.Context, client backstage.Client) error {
				return runCatalogOverview(c.OutOrStdout(), ctx, client, opts.asJSON)
			})
		},
	}

	root.PersistentFlags().BoolVar(&opts.asJSON, "json", false, "Emit the result as JSON")
	root.PersistentFlags().StringVar(&opts.url, "url", "",
		"Backstage address (default $BACKSTAGE_BASE_URL, then $BACKSTAGE_URL)")
	root.PersistentFlags().StringVar(&opts.vaultSecret, "vault-secret", "",
		"Vault KV path holding the Backstage url and token (default $BACKSTAGE_VAULT_SECRET)")
	root.PersistentFlags().StringVar(&opts.vaultAuthPath, "vault-auth-path", "github",
		"Mount path of Vault's GitHub auth method, used only when no Vault token exists")

	root.AddCommand(
		newBackstageEntitiesCmd(deps, &opts),
		newBackstageOfertasCmd(deps, &opts),
		newBackstageCreateCmd(deps, &opts),
		newBackstageGetCmd(deps, &opts),
		newBackstageRepoCmd(deps, &opts),
	)

	return root
}

// backstageOpts carries the flags shared by every backstage subcommand.
type backstageOpts struct {
	asJSON        bool
	url           string
	vaultSecret   string
	vaultAuthPath string
}

// withBackstage builds a client and hands it to fn, turning the two ways this
// can be misconfigured into advice rather than a bare error.
func withBackstage(c *cobra.Command, deps Deps, opts backstageOpts, fn func(context.Context, backstage.Client) error) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := backstage.Config{BaseURL: opts.url}

	if secretPath := firstNonEmpty(opts.vaultSecret, deps.Getenv("BACKSTAGE_VAULT_SECRET")); secretPath != "" {
		url, token, err := backstageFromVault(ctx, deps, secretPath, opts.vaultAuthPath)
		if err != nil {
			return err
		}
		// --url is the more specific request, so it survives; the token has
		// no flag to compete with and Vault is the more deliberate source
		// than an ambient environment variable, so it wins over one.
		if cfg.BaseURL == "" {
			cfg.BaseURL = url
		}
		cfg.Token = token
	}

	client, err := deps.NewBackstageClient(cfg)
	if err != nil {
		if errors.Is(err, backstage.ErrNoBaseURL) {
			return fmt.Errorf("%w\n\nPass --url, set BACKSTAGE_BASE_URL to your Backstage address,\n"+
				"or point --vault-secret at a Vault secret holding a url field", err)
		}
		return err
	}

	return fn(ctx, client)
}

// backstageFromVault reads the Backstage url and token out of a Vault KV
// secret. Either field may be absent -- a catalog that allows anonymous reads
// needs no token, and the address may come from a flag -- but a secret with
// neither is a misconfiguration worth reporting, because the user asked for
// this path expecting something to come out of it.
//
// The token is returned to the caller and passed to the catalog client. It is
// never printed and never appears in an error: the diagnostics below name
// fields, never values.
func backstageFromVault(ctx context.Context, deps Deps, path, authPath string) (url, token string, err error) {
	client, err := deps.NewVaultClient(ctx, vault.Options{
		GitHubToken: deps.GitHubToken,
		AuthPath:    authPath,
	})
	if err != nil {
		if errors.Is(err, vault.ErrNoToken) {
			return "", "", fmt.Errorf("reading %s: %w\n\nSet VAULT_TOKEN, run `vault login`, or authenticate with\n"+
				"GitHub by running `gh auth login` so this command can log in for you", path, err)
		}
		return "", "", err
	}

	secret, err := client.ReadSecret(ctx, path)
	if err != nil {
		return "", "", err
	}

	url = firstFieldOf(secret, vaultURLFields)
	token = firstFieldOf(secret, vaultTokenFields)

	if url == "" && token == "" {
		return "", "", fmt.Errorf("%s has no %s and no %s field (it has: %s)",
			path,
			strings.Join(quoteAll(vaultURLFields), " or "),
			strings.Join(quoteAll(vaultTokenFields), " or "),
			strings.Join(secret.Keys(), ", "))
	}

	return url, token, nil
}

// firstFieldOf returns the first of names present and non-empty in the secret.
func firstFieldOf(secret vault.Secret, names []string) string {
	for _, name := range names {
		if value := secret.Data[name]; value != "" {
			return value
		}
	}
	return ""
}

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return quoted
}

// firstNonEmpty returns the first value that is not the empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// backstage (overview)
// ---------------------------------------------------------------------------

func runCatalogOverview(w io.Writer, ctx context.Context, client backstage.Client, asJSON bool) error {
	facets, err := client.Facets(ctx, "kind")
	if err != nil {
		return err
	}

	kinds := facets["kind"]
	// Largest first: on a real catalog the tail is a long list of ones.
	sort.SliceStable(kinds, func(i, j int) bool { return kinds[i].Count > kinds[j].Count })

	total := 0
	for _, kind := range kinds {
		total += kind.Count
	}

	if asJSON {
		return encodeJSON(w, map[string]interface{}{
			"url":     client.BaseURL(),
			"total":   total,
			"byKind":  kinds,
			"reached": true,
		})
	}

	fmt.Fprintf(w, "Backstage  %s\n\n", client.BaseURL())

	if len(kinds) == 0 {
		fmt.Fprintln(w, "Catalog is empty, or nothing in it is visible to you.")
		return nil
	}

	fmt.Fprintf(w, "Catalog (%d entities)\n", total)
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, kind := range kinds {
		fmt.Fprintf(tw, "  %s\t%d\n", kind.Value, kind.Count)
	}
	tw.Flush()
	return nil
}

// ---------------------------------------------------------------------------
// backstage entities
// ---------------------------------------------------------------------------

func newBackstageEntitiesCmd(deps Deps, opts *backstageOpts) *cobra.Command {
	var (
		kinds      []string
		types      []string
		lifecycles []string
		owners     []string
		tags       []string
		namespace  string
		rawFilters []string
		search     string
		limit      int
		all        bool
	)

	c := &cobra.Command{
		Use:   "entities",
		Short: "List catalog entities",
		Long: `entities lists what the catalog holds, narrowed by the flags you give.

Flags of the same kind are OR'd -- --kind component --kind api means either --
and different flags are AND'd. --filter takes the catalog's own syntax for
anything the flags do not cover:

  --filter 'metadata.annotations.backstage.io/techdocs-ref'
  --filter 'relations.ownedBy=group:default/platform'

A bare key with no '=' matches entities where that field merely exists.

Results are one page of --limit entities. Pass --all to follow the catalog's
pagination to the end, which on a large instance is many requests.`,
		Example: `  gh cli-extension backstage entities --kind component --type service
  gh cli-extension backstage entities --owner group:default/platform --all
  gh cli-extension backstage entities --search payments --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			filter := backstage.Filter{}
			addValues(filter, "kind", kinds)
			addValues(filter, "spec.type", types)
			addValues(filter, "spec.lifecycle", lifecycles)
			addValues(filter, "spec.owner", owners)
			addValues(filter, "metadata.tags", tags)
			if namespace != "" {
				filter.Add("metadata.namespace", namespace)
			}
			for _, raw := range rawFilters {
				key, value, found := strings.Cut(raw, "=")
				if key == "" {
					return fmt.Errorf("--filter %q has no field name", raw)
				}
				if found {
					filter.Add(key, value)
				} else {
					filter.Add(key)
				}
			}

			query := backstage.EntityQuery{
				Limit:        limit,
				FullTextTerm: search,
				OrderBy:      []backstage.Order{{Field: "metadata.name"}},
			}
			if len(filter) > 0 {
				query.Filters = []backstage.Filter{filter}
			}

			return withBackstage(c, deps, *opts, func(ctx context.Context, client backstage.Client) error {
				if all {
					entities, err := client.AllEntities(ctx, query)
					if err != nil {
						return err
					}
					return writeEntityList(c.OutOrStdout(), backstage.EntityPage{
						Items: entities, TotalItems: len(entities),
					}, opts.asJSON)
				}

				page, err := client.Entities(ctx, query)
				if err != nil {
					return err
				}
				return writeEntityList(c.OutOrStdout(), page, opts.asJSON)
			})
		},
	}

	c.Flags().StringSliceVar(&kinds, "kind", nil, "Only this kind (component, api, group, ...); repeatable")
	c.Flags().StringSliceVar(&types, "type", nil, "Only this spec.type (service, website, library, ...); repeatable")
	c.Flags().StringSliceVar(&lifecycles, "lifecycle", nil, "Only this spec.lifecycle (production, experimental, ...); repeatable")
	c.Flags().StringSliceVar(&owners, "owner", nil, "Only entities with this spec.owner; repeatable")
	c.Flags().StringSliceVar(&tags, "tag", nil, "Only entities carrying this tag; repeatable")
	c.Flags().StringVar(&namespace, "namespace", "", "Only entities in this namespace")
	c.Flags().StringArrayVar(&rawFilters, "filter", nil, "Raw catalog filter, 'field=value' or 'field'; repeatable")
	c.Flags().StringVar(&search, "search", "", "Full-text search across the entity")
	c.Flags().IntVar(&limit, "limit", backstage.DefaultPageSize, "Maximum entities to return")
	c.Flags().BoolVar(&all, "all", false, "Follow pagination and return every match")

	return c
}

func writeEntityList(w io.Writer, page backstage.EntityPage, asJSON bool) error {
	if asJSON {
		items := page.Items
		if items == nil {
			items = []backstage.Entity{}
		}
		return encodeJSON(w, map[string]interface{}{
			"items":      items,
			"count":      len(items),
			"totalItems": page.TotalItems,
			"nextCursor": page.NextCursor,
		})
	}

	if len(page.Items) == 0 {
		fmt.Fprintln(w, "No entities match.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, entity := range page.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			entity.Ref(), orDash(entity.Type()), orDash(entity.Lifecycle()), orDash(entity.Owner()))
	}
	tw.Flush()

	// Say so when the answer is partial: a silently truncated list is worse
	// than a long one.
	if page.NextCursor != "" {
		fmt.Fprintf(w, "\nShowing %d", len(page.Items))
		if page.TotalItems > len(page.Items) {
			fmt.Fprintf(w, " of %d", page.TotalItems)
		}
		fmt.Fprintln(w, "; --all for the rest")
	}
	return nil
}

// ---------------------------------------------------------------------------
// backstage ofertas
// ---------------------------------------------------------------------------

// templateField is one input a Template asks for before it will run.
type templateField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// templateOffer is a Template reduced to what someone choosing between them
// needs: what it is called, what it does, and what it will ask for.
type templateOffer struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Fields      []templateField `json:"fields"`
}

func newBackstageOfertasCmd(deps Deps, opts *backstageOpts) *cobra.Command {
	return &cobra.Command{
		Use:     "ofertas",
		Aliases: []string{"offerings", "templates"},
		Short:   "List the scaffolder templates and the inputs each one asks for",
		Long: `ofertas lists every Template in the catalog -- what a developer can actually
ask the portal to create -- together with the inputs each one requires.

The inputs come from the template's spec.parameters, flattened across its form
pages into one list. Each carries a name, a description and whether it is
required. A field with no description falls back to its form label, which is
what the parameter schema calls "title".

The order is the order of the form pages; within a page, required fields come
first and then alphabetical. The catalog's own ordering within a page is not
recoverable -- a JSON object has no order once decoded.

Use --json for the full structure.`,
		Example: `  gh cli-extension backstage ofertas
  gh cli-extension backstage ofertas --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			query := backstage.EntityQuery{
				Filters: []backstage.Filter{backstage.Filter{}.Add("kind", "template")},
				OrderBy: []backstage.Order{{Field: "metadata.name"}},
			}

			return withBackstage(c, deps, *opts, func(ctx context.Context, client backstage.Client) error {
				entities, err := client.AllEntities(ctx, query)
				if err != nil {
					return err
				}

				offers := make([]templateOffer, 0, len(entities))
				for _, entity := range entities {
					offers = append(offers, templateOffer{
						Name:        entity.Metadata.Name,
						Title:       entity.Metadata.Title,
						Description: oneLine(entity.Metadata.Description),
						Fields:      templateFields(entity),
					})
				}

				if opts.asJSON {
					return encodeJSON(c.OutOrStdout(), map[string]interface{}{
						"count":     len(offers),
						"templates": offers,
					})
				}
				writeOfertas(c.OutOrStdout(), offers)
				return nil
			})
		},
	}
}

// templateFields flattens a Template's spec.parameters into one list.
//
// The schema is a JSON Schema fragment per form page: a "properties" object of
// name to sub-schema, and a "required" array naming the mandatory ones.
// spec.parameters is normally an array of those pages, but a single-page
// template may store the object directly, so both shapes are accepted.
func templateFields(entity backstage.Entity) []templateField {
	fields := []templateField{}

	for _, page := range parameterPages(entity.Spec["parameters"]) {
		properties, _ := page["properties"].(map[string]interface{})
		if len(properties) == 0 {
			continue
		}

		required := map[string]bool{}
		if names, ok := page["required"].([]interface{}); ok {
			for _, name := range names {
				if text, ok := name.(string); ok {
					required[text] = true
				}
			}
		}

		var pageFields []templateField
		for name, schema := range properties {
			pageFields = append(pageFields, templateField{
				Name:        name,
				Description: fieldDescription(schema),
				Required:    required[name],
			})
		}

		// A map has no order, so the catalog's own field order is already
		// lost by the time it reaches here. Required first, then alphabetical:
		// deterministic, and it puts what you must supply at the top.
		sort.SliceStable(pageFields, func(i, j int) bool {
			if pageFields[i].Required != pageFields[j].Required {
				return pageFields[i].Required
			}
			return pageFields[i].Name < pageFields[j].Name
		})
		fields = append(fields, pageFields...)
	}

	return fields
}

// templateSchemas returns each parameter's raw JSON Schema, keyed by name,
// flattened across the form pages. `create` needs it to coerce a string from
// the command line into the type the parameter declares.
func templateSchemas(entity backstage.Entity) map[string]map[string]interface{} {
	schemas := map[string]map[string]interface{}{}

	for _, page := range parameterPages(entity.Spec["parameters"]) {
		properties, _ := page["properties"].(map[string]interface{})
		for name, schema := range properties {
			if object, ok := schema.(map[string]interface{}); ok {
				schemas[name] = object
			} else {
				// Present but unreadable: still a valid parameter name, just
				// one we cannot type-check.
				schemas[name] = map[string]interface{}{}
			}
		}
	}
	return schemas
}

// parameterPages normalises spec.parameters into a list of pages.
func parameterPages(raw interface{}) []map[string]interface{} {
	switch typed := raw.(type) {
	case []interface{}:
		pages := make([]map[string]interface{}, 0, len(typed))
		for _, page := range typed {
			if object, ok := page.(map[string]interface{}); ok {
				pages = append(pages, object)
			}
		}
		return pages
	case map[string]interface{}:
		return []map[string]interface{}{typed}
	default:
		return nil
	}
}

// fieldDescription prefers the schema's description and falls back to its
// title, which is the form label -- many parameters carry only that, and a
// field with no text at all tells the reader nothing.
func fieldDescription(schema interface{}) string {
	object, ok := schema.(map[string]interface{})
	if !ok {
		return ""
	}
	if description, ok := object["description"].(string); ok && description != "" {
		return oneLine(description)
	}
	title, _ := object["title"].(string)
	return oneLine(title)
}

func writeOfertas(w io.Writer, offers []templateOffer) {
	if len(offers) == 0 {
		fmt.Fprintln(w, "No templates in the catalog.")
		return
	}

	for i, offer := range offers {
		if i > 0 {
			fmt.Fprintln(w)
		}

		header := offer.Name
		if offer.Title != "" && offer.Title != offer.Name {
			header += "  " + offer.Title
		}
		fmt.Fprintln(w, header)
		if offer.Description != "" {
			fmt.Fprintf(w, "  %s\n", offer.Description)
		}

		if len(offer.Fields) == 0 {
			fmt.Fprintln(w, "\n  (asks for nothing)")
			continue
		}

		fmt.Fprintln(w)
		tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
		for _, field := range offer.Fields {
			marker := " "
			if field.Required {
				marker = "*"
			}
			fmt.Fprintf(tw, "  %s %s\t%s\n", marker, field.Name, field.Description)
		}
		tw.Flush()
	}

	fmt.Fprintln(w, "\n* required")
}

// ---------------------------------------------------------------------------
// backstage create
// ---------------------------------------------------------------------------

// taskPollInterval is how often a run is polled while waiting. The scaffolder
// has no long-poll, so this is a plain loop; a second is short enough to feel
// live and long enough not to hammer a portal.
const taskPollInterval = time.Second

func newBackstageCreateCmd(deps Deps, opts *backstageOpts) *cobra.Command {
	var (
		fields  []string
		dryRun  bool
		noWait  bool
		timeout time.Duration
	)

	c := &cobra.Command{
		Use:   "create <template> [--field name=value]",
		Short: "Run a scaffolder template",
		Long: `create runs a Template, which is how the portal turns a template into a real
repository. Use 'backstage ofertas' to see what each one asks for.

The template is named the short way or in full:

  gh cli-extension backstage create terraform-module --field name=s3-bucket
  gh cli-extension backstage create template:default/terraform-module ...

Values are given one --field at a time. They are checked against the
template's own schema before anything is submitted, so a misspelled parameter
or a missing required one fails locally rather than as a failed run in someone
else's portal. Strings arrive as strings; a parameter the schema declares as a
boolean, an integer or a number is converted, and one that expects an array or
an object is rejected -- pass those through the portal.

A repoUrl parameter uses the picker's own encoding, not a plain URL:

  --field repoUrl=github.com?owner=acme&repo=payments

This is the one command in this tree that changes something outside your
machine: it creates a task, and a successful task creates a repository. Nothing
about it is undone by interrupting it. --dry-run resolves and validates the
values and prints what would be sent, without sending it.

By default the run is followed until it finishes and its log is printed.
--no-wait submits and returns the task id instead.`,
		Example: `  gh cli-extension backstage create techdocs-site --field name=runbooks --dry-run
  gh cli-extension backstage create terraform-module --field name=s3-bucket --field owner=group:default/guests --field provider=aws
  gh cli-extension backstage create svc --field 'repoUrl=github.com?owner=acme&repo=payments' --no-wait`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ref, err := backstage.ParseEntityRef(args[0], "template")
			if err != nil {
				return err
			}

			given, err := parseFieldFlags(fields)
			if err != nil {
				return err
			}

			return withBackstage(c, deps, *opts, func(ctx context.Context, client backstage.Client) error {
				template, err := client.Entity(ctx, ref)
				if err != nil {
					return err
				}

				values, err := resolveTemplateValues(template, given)
				if err != nil {
					return err
				}

				out := c.OutOrStdout()

				if dryRun {
					if opts.asJSON {
						return encodeJSON(out, map[string]interface{}{
							"templateRef": ref.String(),
							"values":      values,
							"submitted":   false,
						})
					}
					fmt.Fprintf(out, "%s\n\nWould submit:\n", ref)
					writeValues(out, values)
					fmt.Fprintln(out, "\nNothing was sent. Drop --dry-run to run it.")
					return nil
				}

				task, err := client.Scaffold(ctx, ref, values)
				if err != nil {
					return err
				}

				if noWait {
					if opts.asJSON {
						return encodeJSON(out, task)
					}
					fmt.Fprintf(out, "%s\n%s\n", task.ID, task.URL)
					return nil
				}

				return followTask(ctx, out, client, task, timeout, opts.asJSON)
			})
		},
	}

	c.Flags().StringArrayVar(&fields, "field", nil, "Template input as name=value; repeatable")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print what would be sent, without sending it")
	c.Flags().BoolVar(&noWait, "no-wait", false, "Submit and print the task id instead of following the run")
	c.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "How long to follow the run before giving up on it")

	return c
}

// parseFieldFlags turns --field name=value into a map, in order, rejecting the
// shapes that are always a mistake.
func parseFieldFlags(fields []string) (map[string]string, error) {
	given := make(map[string]string, len(fields))

	for _, raw := range fields {
		name, value, found := strings.Cut(raw, "=")
		if !found {
			return nil, fmt.Errorf("--field %q needs a value, written name=value", raw)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("--field %q has no name", raw)
		}
		if _, repeated := given[name]; repeated {
			return nil, fmt.Errorf("--field %s was given twice", name)
		}
		given[name] = value
	}
	return given, nil
}

// resolveTemplateValues checks the given values against the template's schema
// and converts each to the type the parameter declares.
func resolveTemplateValues(template backstage.Entity, given map[string]string) (map[string]interface{}, error) {
	schemas := templateSchemas(template)

	// Unknown names first: a typo should be reported as a typo, not as a
	// missing required field somewhere else.
	var unknown []string
	for name := range given {
		if _, ok := schemas[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%s has no parameter %s (it takes: %s)",
			template.Metadata.Name,
			strings.Join(quoteAll(unknown), ", "),
			strings.Join(parameterNames(schemas), ", "))
	}

	values := map[string]interface{}{}
	for name, raw := range given {
		value, err := coerceValue(name, raw, schemas[name])
		if err != nil {
			return nil, err
		}
		values[name] = value
	}

	var missing []string
	for _, field := range templateFields(template) {
		if field.Required {
			if _, ok := values[field.Name]; !ok {
				missing = append(missing, field.Name)
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s requires %s",
			template.Metadata.Name, strings.Join(quoteAll(missing), ", "))
	}

	return values, nil
}

func parameterNames(schemas map[string]map[string]interface{}) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// coerceValue converts a command-line string to what the schema declares.
// Everything arrives as a string; sending "true" where the template expects a
// boolean is rejected by the scaffolder with a message that does not mention
// the quoting, so it is converted here instead.
func coerceValue(name, raw string, schema map[string]interface{}) (interface{}, error) {
	declared, _ := schema["type"].(string)

	switch declared {
	case "boolean":
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("--field %s expects a boolean, got %q", name, raw)
		}
		return parsed, nil
	case "integer":
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--field %s expects an integer, got %q", name, raw)
		}
		return parsed, nil
	case "number":
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("--field %s expects a number, got %q", name, raw)
		}
		return parsed, nil
	case "array", "object":
		return nil, fmt.Errorf("--field %s expects %s, which this command cannot express; run this template from the portal",
			name, declared)
	default:
		// "string", or a schema that declares nothing. Passing the string
		// through is right either way.
		return raw, nil
	}
}

func writeValues(w io.Writer, values map[string]interface{}) {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, name := range names {
		fmt.Fprintf(tw, "  %s\t%v\n", name, values[name])
	}
	tw.Flush()
}

// followTask prints a run's log as it arrives and returns when it ends.
//
// A failed run returns a SilentError: its log has already been printed, and
// the entrypoint should set the exit code without adding a second message.
func followTask(ctx context.Context, w io.Writer, client backstage.Client, task backstage.Task, timeout time.Duration, asJSON bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if !asJSON {
		fmt.Fprintf(w, "%s\n%s\n\n", task.ID, task.URL)
	}

	var (
		after   int
		lines   []string
		outputs map[string]interface{}
		failure string
	)

	for {
		events, err := client.TaskEvents(ctx, task.ID, after)
		if err != nil {
			return err
		}

		for _, event := range events {
			if event.ID > after {
				after = event.ID
			}
			if event.Message != "" {
				lines = append(lines, event.Message)
				if !asJSON {
					fmt.Fprintln(w, event.Message)
				}
			}
			if event.Type == "completion" {
				outputs, failure = event.Output, event.Error
			}
		}

		current, err := client.Task(ctx, task.ID)
		if err != nil {
			return err
		}
		task = current

		if task.Done() {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("task %s is still %s after %s; follow it at %s",
				task.ID, task.Status, timeout, task.URL)
		case <-time.After(taskPollInterval):
		}
	}

	if asJSON {
		payload := map[string]interface{}{
			"id": task.ID, "status": task.Status, "url": task.URL,
			"templateRef": task.TemplateRef, "log": lines,
		}
		if outputs != nil {
			payload["output"] = outputs
		}
		if failure != "" {
			payload["error"] = failure
		}
		if err := encodeJSON(w, payload); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "\n%s\n", task.Status)
		if failure != "" {
			fmt.Fprintf(w, "  %s\n", failure)
		}
		writeTaskOutput(w, outputs)
	}

	if task.Failed() {
		return SilentError{Err: fmt.Errorf("task %s %s", task.ID, task.Status)}
	}
	return nil
}

// writeTaskOutput renders the template's declared output, which is a list of
// links often enough to be worth handling specially.
func writeTaskOutput(w io.Writer, output map[string]interface{}) {
	if len(output) == 0 {
		return
	}

	links, _ := output["links"].([]interface{})
	if len(links) == 0 {
		fmt.Fprintln(w, "\nOutput")
		writeValues(w, output)
		return
	}

	fmt.Fprintln(w, "\nOutput")
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, raw := range links {
		link, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := link["title"].(string)
		target, _ := link["url"].(string)
		if target == "" {
			target, _ = link["entityRef"].(string)
		}
		fmt.Fprintf(tw, "  %s\t%s\n", title, target)
	}
	tw.Flush()
}

// ---------------------------------------------------------------------------
// backstage get
// ---------------------------------------------------------------------------

func newBackstageGetCmd(deps Deps, opts *backstageOpts) *cobra.Command {
	var kind string

	c := &cobra.Command{
		Use:   "get <entity>",
		Short: "Show one catalog entity",
		Long: `get fetches a single entity by reference.

A reference is [<kind>:][<namespace>/]<name>, and the namespace defaults to
'default':

  component:default/payments
  component:payments
  payments --kind component

Use --kind when you would rather not type the prefix.`,
		Example: `  gh cli-extension backstage get component:payments
  gh cli-extension backstage get group:platform/team-a --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ref, err := backstage.ParseEntityRef(args[0], kind)
			if err != nil {
				return err
			}

			return withBackstage(c, deps, *opts, func(ctx context.Context, client backstage.Client) error {
				entity, err := client.Entity(ctx, ref)
				if err != nil {
					return err
				}
				if opts.asJSON {
					return encodeJSON(c.OutOrStdout(), entity)
				}
				writeEntity(c.OutOrStdout(), entity)
				return nil
			})
		},
	}

	c.Flags().StringVar(&kind, "kind", "", "Kind to assume when the reference omits it")

	return c
}

func writeEntity(w io.Writer, entity backstage.Entity) {
	header := entity.Ref().String()
	if title := entity.Metadata.Title; title != "" && title != entity.Metadata.Name {
		header += "  " + title
	}
	fmt.Fprintf(w, "%s\n\n", header)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	row := func(label, value string) {
		if value != "" {
			fmt.Fprintf(tw, "  %s\t%s\n", label, value)
		}
	}
	row("kind", entity.Kind)
	row("namespace", entity.Ref().Namespace)
	row("type", entity.Type())
	row("lifecycle", entity.Lifecycle())
	row("owner", entity.Owner())
	row("system", entity.SpecString("system"))
	row("description", oneLine(entity.Metadata.Description))
	row("tags", strings.Join(entity.Metadata.Tags, ", "))
	row("source", entity.Annotation("backstage.io/source-location"))
	row("repository", entity.Annotation(projectSlugAnnotation))
	tw.Flush()

	if len(entity.Relations) == 0 {
		return
	}

	// Grouped by type: a busy entity has a dozen dependsOn edges and reading
	// them interleaved with ownedBy is pointless.
	fmt.Fprintln(w, "\nRelations")
	relations := map[string][]string{}
	var order []string
	for _, relation := range entity.Relations {
		if _, seen := relations[relation.Type]; !seen {
			order = append(order, relation.Type)
		}
		relations[relation.Type] = append(relations[relation.Type], relation.TargetRef)
	}
	sort.Strings(order)

	tw = tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, relationType := range order {
		for i, target := range relations[relationType] {
			label := relationType
			if i > 0 {
				label = ""
			}
			fmt.Fprintf(tw, "  %s\t%s\n", label, target)
		}
	}
	tw.Flush()
}

// ---------------------------------------------------------------------------
// backstage repo
// ---------------------------------------------------------------------------

func newBackstageRepoCmd(deps Deps, opts *backstageOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "repo",
		Short: "Show the catalog entities for the current repository",
		Long: `repo resolves the repository you are standing in, the same way 'gh' does,
and looks for catalog entities annotated with it.

The link is the ` + projectSlugAnnotation + ` annotation, which Backstage's
GitHub integrations write when they discover a catalog-info.yaml. An entity
registered without that annotation will not be found here even though it
describes this repository -- there is nothing in the catalog tying the two
together in that case.

A repository can hold several entities: a component, the APIs it exposes, the
resources it owns.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := deps.CurrentRepo()
			if err != nil {
				return err
			}
			slug := repo.String()

			query := backstage.EntityQuery{
				Filters: []backstage.Filter{
					backstage.Filter{}.Add("metadata.annotations."+projectSlugAnnotation, slug),
				},
				OrderBy: []backstage.Order{{Field: "metadata.name"}},
			}

			return withBackstage(c, deps, *opts, func(ctx context.Context, client backstage.Client) error {
				entities, err := client.AllEntities(ctx, query)
				if err != nil {
					return err
				}

				if opts.asJSON {
					if entities == nil {
						entities = []backstage.Entity{}
					}
					return encodeJSON(c.OutOrStdout(), map[string]interface{}{
						"repository": slug,
						"items":      entities,
						"count":      len(entities),
					})
				}

				out := c.OutOrStdout()
				fmt.Fprintf(out, "%s\n\n", slug)
				if len(entities) == 0 {
					fmt.Fprintf(out, "Not in the catalog: no entity is annotated %s=%s\n",
						projectSlugAnnotation, slug)
					return nil
				}
				return writeEntityList(out, backstage.EntityPage{
					Items: entities, TotalItems: len(entities),
				}, false)
			})
		},
	}
}

// oneLine flattens a value onto a single line. Catalog descriptions are YAML
// block scalars and routinely carry newlines; printed as-is they break the
// tabwriter's column block, so every row after them is misaligned.
func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// addValues adds a flag's values to a filter, skipping the key entirely when
// the flag was not given. Filter.Add with no values means "this field exists",
// which is a real filter and not what an unset flag should produce.
func addValues(filter backstage.Filter, key string, values []string) {
	if len(values) > 0 {
		filter.Add(key, values...)
	}
}

// orDash keeps a column aligned when an entity leaves a field unset.
func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
