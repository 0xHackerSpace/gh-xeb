package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

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
