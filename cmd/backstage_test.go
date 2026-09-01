package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/backstage"
	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
	"github.com/0xHackerSpace/gh-cli-extension/internal/vault"
)

func component(name, kind, specType, lifecycle, owner string) backstage.Entity {
	return backstage.Entity{
		Kind:     kind,
		Metadata: backstage.EntityMetadata{Name: name, Namespace: "default"},
		Spec: map[string]interface{}{
			"type": specType, "lifecycle": lifecycle, "owner": owner,
		},
	}
}

// filterOf returns the single filter clause of the query at index i, so a test
// can assert on what a command asked the catalog for.
func filterOf(t *testing.T, fake *fakeBackstage, i int) backstage.Filter {
	t.Helper()
	if len(fake.queries) <= i {
		t.Fatalf("only %d queries were made, wanted at least %d", len(fake.queries), i+1)
	}
	filters := fake.queries[i].Filters
	if len(filters) != 1 {
		t.Fatalf("query %d has %d filter clauses, want exactly 1", i, len(filters))
	}
	return filters[0]
}

func TestBackstageOverview(t *testing.T) {
	fake := &fakeBackstage{
		addr: "https://backstage.acme.dev",
		facets: map[string][]backstage.Facet{
			"kind": {{Value: "API", Count: 7}, {Value: "Component", Count: 42}},
		},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage")
	if err != nil {
		t.Fatalf("backstage: %v", err)
	}

	for _, want := range []string{"https://backstage.acme.dev", "49 entities", "Component", "42"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Sorted by count, so the biggest kind leads.
	if strings.Index(out, "Component") > strings.Index(out, "API") {
		t.Errorf("kinds are not ordered by count:\n%s", out)
	}
}

func TestBackstageOverviewEmptyCatalog(t *testing.T) {
	out, err := runRoot(t, backstageDeps(&fakeBackstage{}), "backstage")
	if err != nil {
		t.Fatalf("backstage: %v", err)
	}
	if !strings.Contains(out, "empty") {
		t.Errorf("output does not say the catalog is empty:\n%s", out)
	}
}

func TestBackstageOverviewJSON(t *testing.T) {
	fake := &fakeBackstage{
		facets: map[string][]backstage.Facet{"kind": {{Value: "Component", Count: 3}}},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "--json")
	if err != nil {
		t.Fatalf("backstage --json: %v", err)
	}

	var payload struct {
		URL    string `json:"url"`
		Total  int    `json:"total"`
		ByKind []struct {
			Value string `json:"value"`
			Count int    `json:"count"`
		} `json:"byKind"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decoding %q: %v", out, err)
	}
	if payload.Total != 3 || len(payload.ByKind) != 1 || payload.ByKind[0].Value != "Component" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBackstageWithoutAnAddressExplainsHow(t *testing.T) {
	deps := testDeps()
	deps.NewBackstageClient = func(backstage.Config) (backstage.Client, error) {
		return nil, backstage.ErrNoBaseURL
	}

	_, err := runRoot(t, deps, "backstage")
	if err == nil {
		t.Fatal("want an error when no address is configured")
	}
	if !errors.Is(err, backstage.ErrNoBaseURL) {
		t.Errorf("error does not wrap ErrNoBaseURL: %v", err)
	}
	if !strings.Contains(err.Error(), "BACKSTAGE_BASE_URL") || !strings.Contains(err.Error(), "--url") {
		t.Errorf("error does not say how to fix it: %v", err)
	}
}

func TestBackstageURLFlagReachesTheConstructor(t *testing.T) {
	var got backstage.Config
	deps := testDeps()
	deps.NewBackstageClient = func(cfg backstage.Config) (backstage.Client, error) {
		got = cfg
		return &fakeBackstage{}, nil
	}

	if _, err := runRoot(t, deps, "backstage", "--url", "https://from-flag.example.com"); err != nil {
		t.Fatalf("backstage --url: %v", err)
	}
	if got.BaseURL != "https://from-flag.example.com" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.Token != "" {
		t.Errorf("the command set a token (%q); it must come from the environment only", got.Token)
	}
}

func TestBackstageHasNoTokenFlag(t *testing.T) {
	// A token on the command line lands in shell history and in ps. If this
	// test ever fails, that decision was reversed by accident.
	root := NewRootCmd(testDeps())
	for _, cmd := range root.Commands() {
		if cmd.Name() != "backstage" {
			continue
		}
		if flag := cmd.PersistentFlags().Lookup("token"); flag != nil {
			t.Fatal("backstage grew a --token flag")
		}
		for _, sub := range cmd.Commands() {
			if flag := sub.Flags().Lookup("token"); flag != nil {
				t.Fatalf("backstage %s grew a --token flag", sub.Name())
			}
		}
		return
	}
	t.Fatal("no backstage command is registered")
}

func TestBackstageEntities(t *testing.T) {
	fake := &fakeBackstage{
		page: backstage.EntityPage{
			Items: []backstage.Entity{
				component("payments", "Component", "service", "production", "group:default/pay"),
				component("docs", "Component", "website", "", ""),
			},
			TotalItems: 2,
		},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "entities")
	if err != nil {
		t.Fatalf("backstage entities: %v", err)
	}

	for _, want := range []string{"component:default/payments", "service", "production", "group:default/pay"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Unset fields become a dash so the columns stay readable.
	if !strings.Contains(out, "component:default/docs") || !strings.Contains(out, "-") {
		t.Errorf("output does not pad the unset fields:\n%s", out)
	}
	if len(fake.queries) != 1 {
		t.Fatalf("made %d queries, want 1", len(fake.queries))
	}
	if fake.queries[0].Filters != nil {
		t.Errorf("an unfiltered listing sent a filter: %v", fake.queries[0].Filters)
	}
}

func TestBackstageEntitiesBuildsTheFilter(t *testing.T) {
	fake := &fakeBackstage{}

	_, err := runRoot(t, backstageDeps(fake), "backstage", "entities",
		"--kind", "component", "--kind", "api",
		"--type", "service",
		"--lifecycle", "production",
		"--owner", "group:default/platform",
		"--tag", "go",
		"--namespace", "payments",
		"--filter", "relations.ownedBy=group:default/platform",
		"--filter", "metadata.annotations.backstage.io/techdocs-ref",
		"--search", "ledger",
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("backstage entities: %v", err)
	}

	filter := filterOf(t, fake, 0)
	cases := map[string][]string{
		"kind":               {"component", "api"},
		"spec.type":          {"service"},
		"spec.lifecycle":     {"production"},
		"spec.owner":         {"group:default/platform"},
		"metadata.tags":      {"go"},
		"metadata.namespace": {"payments"},
		"relations.ownedBy":  {"group:default/platform"},
		"metadata.annotations.backstage.io/techdocs-ref": nil,
	}
	for key, want := range cases {
		got, present := filter[key]
		if !present {
			t.Errorf("filter has no %q: %v", key, filter)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("filter[%q] = %v, want %v", key, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("filter[%q] = %v, want %v", key, got, want)
				break
			}
		}
	}

	query := fake.queries[0]
	if query.Limit != 5 || query.FullTextTerm != "ledger" {
		t.Errorf("query = %+v", query)
	}
	if len(query.OrderBy) != 1 || query.OrderBy[0].Field != "metadata.name" {
		t.Errorf("OrderBy = %+v, want a stable sort by name", query.OrderBy)
	}
}

func TestBackstageEntitiesRejectsAFilterWithNoField(t *testing.T) {
	fake := &fakeBackstage{}

	_, err := runRoot(t, backstageDeps(fake), "backstage", "entities", "--filter", "=value")
	if err == nil {
		t.Fatal("want an error for a filter with no field name")
	}
	if len(fake.queries) != 0 {
		t.Errorf("made %d queries, want none", len(fake.queries))
	}
}

func TestBackstageEntitiesSaysWhenTheListIsPartial(t *testing.T) {
	fake := &fakeBackstage{
		page: backstage.EntityPage{
			Items:      []backstage.Entity{component("a", "Component", "", "", "")},
			TotalItems: 90,
			NextCursor: "more",
		},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "entities")
	if err != nil {
		t.Fatalf("backstage entities: %v", err)
	}
	if !strings.Contains(out, "Showing 1 of 90") || !strings.Contains(out, "--all") {
		t.Errorf("output does not admit the list is truncated:\n%s", out)
	}
}

func TestBackstageEntitiesAll(t *testing.T) {
	fake := &fakeBackstage{
		// Set on the paginated path only, so a test failure here means --all
		// called Entities instead of AllEntities.
		entities: []backstage.Entity{
			component("a", "Component", "", "", ""),
			component("b", "Component", "", "", ""),
		},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "entities", "--all")
	if err != nil {
		t.Fatalf("backstage entities --all: %v", err)
	}
	if !strings.Contains(out, "component:default/a") || !strings.Contains(out, "component:default/b") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "--all for the rest") {
		t.Errorf("a complete listing still offered --all:\n%s", out)
	}
}

func TestBackstageEntitiesNoMatch(t *testing.T) {
	out, err := runRoot(t, backstageDeps(&fakeBackstage{}), "backstage", "entities", "--kind", "nope")
	if err != nil {
		t.Fatalf("backstage entities: %v", err)
	}
	if !strings.Contains(out, "No entities match") {
		t.Errorf("output:\n%s", out)
	}
}

func TestBackstageEntitiesJSON(t *testing.T) {
	fake := &fakeBackstage{
		page: backstage.EntityPage{
			Items:      []backstage.Entity{component("payments", "Component", "service", "production", "team")},
			TotalItems: 1,
		},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "entities", "--json")
	if err != nil {
		t.Fatalf("backstage entities --json: %v", err)
	}

	var payload struct {
		Count int `json:"count"`
		Items []struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decoding %q: %v", out, err)
	}
	if payload.Count != 1 || payload.Items[0].Metadata.Name != "payments" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBackstageEntitiesJSONWithNoMatchesIsAnArray(t *testing.T) {
	out, err := runRoot(t, backstageDeps(&fakeBackstage{}), "backstage", "entities", "--json")
	if err != nil {
		t.Fatalf("backstage entities --json: %v", err)
	}
	// An empty result must be [] and not null, so a consumer can iterate it.
	if !strings.Contains(out, `"items": []`) {
		t.Errorf("empty listing is not an empty array:\n%s", out)
	}
}

func TestBackstageGet(t *testing.T) {
	entity := backstage.Entity{
		Kind: "Component",
		Metadata: backstage.EntityMetadata{
			Name: "payments", Namespace: "default", Title: "Payments API",
			Description: "Takes the money", Tags: []string{"go", "tier-1"},
			Annotations: map[string]string{projectSlugAnnotation: "acme/payments"},
		},
		Spec: map[string]interface{}{"type": "service", "lifecycle": "production", "owner": "group:default/pay"},
		Relations: []backstage.Relation{
			{Type: "dependsOn", TargetRef: "resource:default/db"},
			{Type: "ownedBy", TargetRef: "group:default/pay"},
			{Type: "dependsOn", TargetRef: "component:default/ledger"},
		},
	}
	fake := &fakeBackstage{byRef: map[string]backstage.Entity{"component:default/payments": entity}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "get", "component:payments")
	if err != nil {
		t.Fatalf("backstage get: %v", err)
	}

	for _, want := range []string{
		"component:default/payments", "Payments API", "service", "production",
		"Takes the money", "go, tier-1", "acme/payments", "Relations",
		"resource:default/db", "component:default/ledger", "group:default/pay",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Relations are grouped, so the type is printed once per group.
	if strings.Count(out, "dependsOn") != 1 {
		t.Errorf("dependsOn is not grouped:\n%s", out)
	}
}

func TestBackstageGetFlattensAMultilineDescription(t *testing.T) {
	fake := &fakeBackstage{byRef: map[string]backstage.Entity{
		"template:default/tf": {
			Kind: "Template",
			Metadata: backstage.EntityMetadata{
				Name: "tf", Namespace: "default",
				Description: "Reusable module\nwith CI validation\n",
				Tags:        []string{"terraform"},
			},
		},
	}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "get", "template:tf")
	if err != nil {
		t.Fatalf("backstage get: %v", err)
	}

	if !strings.Contains(out, "Reusable module with CI validation") {
		t.Errorf("the description was not flattened:\n%s", out)
	}
	// The rows must stay in one aligned block: a newline inside a cell ends
	// the tabwriter's column run and misaligns everything after it.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var description, tags string
	for _, line := range lines {
		switch {
		case strings.Contains(line, "description"):
			description = line
		case strings.Contains(line, "tags"):
			tags = line
		}
	}
	if description == "" || tags == "" {
		t.Fatalf("output is missing a row:\n%s", out)
	}
	if strings.Index(description, "Reusable") != strings.Index(tags, "terraform") {
		t.Errorf("description and tags are not in the same column:\n%s", out)
	}
}

func TestBackstageGetDefaultsTheKind(t *testing.T) {
	fake := &fakeBackstage{
		byRef: map[string]backstage.Entity{
			"group:default/platform": {Kind: "Group", Metadata: backstage.EntityMetadata{Name: "platform"}},
		},
	}

	if _, err := runRoot(t, backstageDeps(fake), "backstage", "get", "platform", "--kind", "group"); err != nil {
		t.Fatalf("backstage get --kind: %v", err)
	}
	if len(fake.refs) != 1 || fake.refs[0].String() != "group:default/platform" {
		t.Fatalf("refs = %v", fake.refs)
	}
}

func TestBackstageGetRejectsARefWithNoKind(t *testing.T) {
	fake := &fakeBackstage{}

	_, err := runRoot(t, backstageDeps(fake), "backstage", "get", "payments")
	if err == nil {
		t.Fatal("want an error for a reference with no kind")
	}
	if !strings.Contains(err.Error(), "component:payments") {
		t.Errorf("error does not show the fixed form: %v", err)
	}
	if len(fake.refs) != 0 {
		t.Errorf("the command called the catalog with an invalid ref")
	}
}

func TestBackstageGetNotFound(t *testing.T) {
	_, err := runRoot(t, backstageDeps(&fakeBackstage{}), "backstage", "get", "component:ghost")
	if !errors.Is(err, backstage.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestBackstageRepo(t *testing.T) {
	fake := &fakeBackstage{
		entities: []backstage.Entity{component("gh-cli-extension", "Component", "tool", "experimental", "team")},
	}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "repo")
	if err != nil {
		t.Fatalf("backstage repo: %v", err)
	}

	if !strings.Contains(out, "0xHackerSpace/gh-cli-extension") {
		t.Errorf("output does not name the repository:\n%s", out)
	}
	if !strings.Contains(out, "component:default/gh-cli-extension") {
		t.Errorf("output:\n%s", out)
	}

	filter := filterOf(t, fake, 0)
	values := filter["metadata.annotations."+projectSlugAnnotation]
	if len(values) != 1 || values[0] != "0xHackerSpace/gh-cli-extension" {
		t.Errorf("filter = %v", filter)
	}
}

func TestBackstageRepoNotRegistered(t *testing.T) {
	out, err := runRoot(t, backstageDeps(&fakeBackstage{}), "backstage", "repo")
	if err != nil {
		t.Fatalf("backstage repo: %v", err)
	}
	if !strings.Contains(out, "Not in the catalog") || !strings.Contains(out, projectSlugAnnotation) {
		t.Errorf("output does not explain the miss:\n%s", out)
	}
}

func TestBackstageRepoOutsideARepository(t *testing.T) {
	deps := backstageDeps(&fakeBackstage{})
	deps.CurrentRepo = func() (gh.Repo, error) { return gh.Repo{}, errors.New("no git remotes found") }

	_, err := runRoot(t, deps, "backstage", "repo")
	if err == nil || !strings.Contains(err.Error(), "no git remotes") {
		t.Fatalf("error = %v, want the repository resolution failure", err)
	}
}

func TestBackstageRepoJSON(t *testing.T) {
	fake := &fakeBackstage{}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "repo", "--json")
	if err != nil {
		t.Fatalf("backstage repo --json: %v", err)
	}

	var payload struct {
		Repository string             `json:"repository"`
		Items      []backstage.Entity `json:"items"`
		Count      int                `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decoding %q: %v", out, err)
	}
	if payload.Repository != "0xHackerSpace/gh-cli-extension" || payload.Count != 0 {
		t.Fatalf("payload = %+v", payload)
	}
	if !strings.Contains(out, `"items": []`) {
		t.Errorf("empty result is not an empty array:\n%s", out)
	}
}

func TestBackstageCatalogErrorsSurface(t *testing.T) {
	boom := errors.New("catalog is down")
	fake := &fakeBackstage{facetsErr: boom, pageErr: boom, allErr: boom, entityErr: boom}
	deps := backstageDeps(fake)

	for _, args := range [][]string{
		{"backstage"},
		{"backstage", "entities"},
		{"backstage", "entities", "--all"},
		{"backstage", "get", "component:x"},
		{"backstage", "repo"},
	} {
		if _, err := runRoot(t, deps, args...); !errors.Is(err, boom) {
			t.Errorf("%v: error = %v, want the catalog failure", args, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Configuration from Vault
// ---------------------------------------------------------------------------

// vaultBackedDeps wires a Backstage fake plus a Vault fake holding one secret,
// and records the Config the Backstage constructor was handed.
func vaultBackedDeps(secrets map[string]vault.Secret, got *backstage.Config) Deps {
	deps := testDeps()
	deps.NewVaultClient = func(context.Context, vault.Options) (vault.Client, error) {
		return fakeVault{secrets: secrets}, nil
	}
	deps.NewBackstageClient = func(cfg backstage.Config) (backstage.Client, error) {
		*got = cfg
		return &fakeBackstage{}, nil
	}
	return deps
}

func kvSecret(path string, fields map[string]string) map[string]vault.Secret {
	return map[string]vault.Secret{path: {Path: path, Mount: "secret/", MountType: "kv-v2", Data: fields}}
}

func TestBackstageReadsURLAndTokenFromVault(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/backstage", map[string]string{
		"url":   "https://backstage.acme.dev",
		"token": "vault-issued",
	}), &got)

	if _, err := runRoot(t, deps, "backstage", "--vault-secret", "secret/backstage"); err != nil {
		t.Fatalf("backstage --vault-secret: %v", err)
	}
	if got.BaseURL != "https://backstage.acme.dev" || got.Token != "vault-issued" {
		t.Fatalf("config = %+v", got)
	}
}

func TestBackstageAcceptsTheAliasFields(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/backstage", map[string]string{
		"base_url":  "https://alias.example.com",
		"api_token": "alias-token",
	}), &got)

	if _, err := runRoot(t, deps, "backstage", "--vault-secret", "secret/backstage"); err != nil {
		t.Fatalf("backstage --vault-secret: %v", err)
	}
	if got.BaseURL != "https://alias.example.com" || got.Token != "alias-token" {
		t.Fatalf("config = %+v", got)
	}
}

func TestBackstageURLFlagBeatsTheVaultSecret(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/backstage", map[string]string{
		"url":   "https://from-vault.example.com",
		"token": "vault-issued",
	}), &got)

	_, err := runRoot(t, deps, "backstage",
		"--vault-secret", "secret/backstage", "--url", "https://from-flag.example.com")
	if err != nil {
		t.Fatalf("backstage: %v", err)
	}
	if got.BaseURL != "https://from-flag.example.com" {
		t.Errorf("BaseURL = %q, want the flag to win", got.BaseURL)
	}
	// The URL being overridden must not cost the token.
	if got.Token != "vault-issued" {
		t.Errorf("Token = %q, want the one from Vault", got.Token)
	}
}

func TestBackstageVaultSecretWithOnlyAToken(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/backstage", map[string]string{
		"token": "vault-issued",
	}), &got)

	// No url anywhere: the empty BaseURL falls through to backstage.New, which
	// reads the environment and is the component that reports it missing.
	if _, err := runRoot(t, deps, "backstage", "--vault-secret", "secret/backstage"); err != nil {
		t.Fatalf("backstage: %v", err)
	}
	if got.BaseURL != "" || got.Token != "vault-issued" {
		t.Fatalf("config = %+v", got)
	}
}

func TestBackstageVaultSecretPathFromTheEnvironment(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/from-env", map[string]string{
		"url": "https://from-env-secret.example.com",
	}), &got)
	deps.Getenv = func(key string) string {
		if key == "BACKSTAGE_VAULT_SECRET" {
			return "secret/from-env"
		}
		return ""
	}

	if _, err := runRoot(t, deps, "backstage"); err != nil {
		t.Fatalf("backstage: %v", err)
	}
	if got.BaseURL != "https://from-env-secret.example.com" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
}

func TestBackstageVaultSecretWithNeitherField(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/backstage", map[string]string{
		"username": "svc", "password": "hunter2",
	}), &got)

	_, err := runRoot(t, deps, "backstage", "--vault-secret", "secret/backstage")
	if err == nil {
		t.Fatal("want an error for a secret carrying neither field")
	}
	// The error must name the fields present so the user can fix the secret...
	for _, want := range []string{"secret/backstage", `"url"`, `"token"`, "username", "password"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
	// ...and must not name their values.
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaked a secret value: %v", err)
	}
	if got.BaseURL != "" || got.Token != "" {
		t.Errorf("the catalog client was built anyway: %+v", got)
	}
}

func TestBackstageVaultSecretMissing(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(nil, &got)

	_, err := runRoot(t, deps, "backstage", "--vault-secret", "secret/absent")
	if err == nil || !strings.Contains(err.Error(), "secret/absent") {
		t.Fatalf("error = %v, want it to name the missing path", err)
	}
}

func TestBackstageWithNoVaultTokenExplainsHow(t *testing.T) {
	deps := testDeps()
	deps.NewVaultClient = func(context.Context, vault.Options) (vault.Client, error) {
		return nil, vault.ErrNoToken
	}

	_, err := runRoot(t, deps, "backstage", "--vault-secret", "secret/backstage")
	if !errors.Is(err, vault.ErrNoToken) {
		t.Fatalf("error = %v, want ErrNoToken", err)
	}
	for _, want := range []string{"secret/backstage", "VAULT_TOKEN", "gh auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
}

func TestBackstagePassesTheAuthPathToVault(t *testing.T) {
	var opts vault.Options
	deps := testDeps()
	deps.NewVaultClient = func(_ context.Context, o vault.Options) (vault.Client, error) {
		opts = o
		return fakeVault{secrets: kvSecret("secret/backstage", map[string]string{"url": "https://x.example.com"})}, nil
	}
	deps.NewBackstageClient = func(backstage.Config) (backstage.Client, error) { return &fakeBackstage{}, nil }

	_, err := runRoot(t, deps, "backstage",
		"--vault-secret", "secret/backstage", "--vault-auth-path", "gh-corp")
	if err != nil {
		t.Fatalf("backstage: %v", err)
	}
	if opts.AuthPath != "gh-corp" {
		t.Errorf("AuthPath = %q", opts.AuthPath)
	}
	if opts.GitHubToken == nil {
		t.Error("the Vault login was given no way to fetch a GitHub token")
	}
}

func TestBackstageDoesNotTouchVaultWithoutBeingAsked(t *testing.T) {
	deps := backstageDeps(&fakeBackstage{})
	deps.NewVaultClient = func(context.Context, vault.Options) (vault.Client, error) {
		t.Fatal("the backstage command contacted Vault without --vault-secret")
		return nil, nil
	}

	for _, args := range [][]string{
		{"backstage"},
		{"backstage", "entities"},
		{"backstage", "get", "component:x"},
		{"backstage", "repo"},
	} {
		if _, err := runRoot(t, deps, args...); err != nil && !errors.Is(err, backstage.ErrNotFound) {
			t.Errorf("%v: %v", args, err)
		}
	}
}

// ---------------------------------------------------------------------------
// backstage ofertas
// ---------------------------------------------------------------------------

// terraformTemplate mirrors the shape a real scaffolder template has: several
// form pages, each with its own properties and required list, and parameters
// that carry only a title rather than a description.
func terraformTemplate() backstage.Entity {
	return backstage.Entity{
		Kind: "Template",
		Metadata: backstage.EntityMetadata{
			Name: "terraform-module", Namespace: "default", Title: "Módulo Terraform",
			Description: "Módulo Terraform reutilizável\ncom exemplo executável.\n",
		},
		Spec: map[string]interface{}{
			"type": "infrastructure",
			"parameters": []interface{}{
				map[string]interface{}{
					"title": "Identificação",
					"properties": map[string]interface{}{
						"name":        map[string]interface{}{"title": "Nome do módulo", "description": "Sem o prefixo terraform-"},
						"owner":       map[string]interface{}{"title": "Owner"},
						"description": map[string]interface{}{"description": "O que provisiona"},
					},
					"required": []interface{}{"name", "owner"},
				},
				map[string]interface{}{
					"title": "Provider",
					"properties": map[string]interface{}{
						"provider":         map[string]interface{}{"title": "Provider principal"},
						"terraformVersion": map[string]interface{}{"title": "Versão mínima"},
					},
					"required": []interface{}{"provider"},
				},
			},
		},
	}
}

func TestBackstageOfertasJSON(t *testing.T) {
	fake := &fakeBackstage{entities: []backstage.Entity{terraformTemplate()}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "ofertas", "--json")
	if err != nil {
		t.Fatalf("backstage ofertas --json: %v", err)
	}

	var payload struct {
		Count     int `json:"count"`
		Templates []struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Fields      []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Required    bool   `json:"required"`
			} `json:"fields"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decoding %q: %v", out, err)
	}

	if payload.Count != 1 || len(payload.Templates) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	tpl := payload.Templates[0]
	if tpl.Name != "terraform-module" || tpl.Title != "Módulo Terraform" {
		t.Errorf("name/title = %q/%q", tpl.Name, tpl.Title)
	}
	if tpl.Description != "Módulo Terraform reutilizável com exemplo executável." {
		t.Errorf("description = %q, want it flattened onto one line", tpl.Description)
	}

	// Page order is kept; within a page, required first then alphabetical.
	want := []struct {
		name, description string
		required          bool
	}{
		{"name", "Sem o prefixo terraform-", true},
		{"owner", "Owner", true},
		{"description", "O que provisiona", false},
		{"provider", "Provider principal", true},
		{"terraformVersion", "Versão mínima", false},
	}
	if len(tpl.Fields) != len(want) {
		t.Fatalf("got %d fields, want %d: %+v", len(tpl.Fields), len(want), tpl.Fields)
	}
	for i, w := range want {
		got := tpl.Fields[i]
		if got.Name != w.name || got.Description != w.description || got.Required != w.required {
			t.Errorf("field %d = %+v, want %v/%q/%v", i, got, w.name, w.description, w.required)
		}
	}
}

func TestBackstageOfertasAsksOnlyForTemplates(t *testing.T) {
	fake := &fakeBackstage{}

	if _, err := runRoot(t, backstageDeps(fake), "backstage", "ofertas"); err != nil {
		t.Fatalf("backstage ofertas: %v", err)
	}

	filter := filterOf(t, fake, 0)
	kinds := filter["kind"]
	if len(kinds) != 1 || kinds[0] != "template" {
		t.Errorf("filter = %v, want kind=template only", filter)
	}
	if len(filter) != 1 {
		t.Errorf("filter carries more than the kind: %v", filter)
	}
}

func TestBackstageOfertasText(t *testing.T) {
	fake := &fakeBackstage{entities: []backstage.Entity{terraformTemplate()}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "ofertas")
	if err != nil {
		t.Fatalf("backstage ofertas: %v", err)
	}

	for _, want := range []string{
		"terraform-module", "Módulo Terraform", "Sem o prefixo terraform-",
		"* name", "* owner", "* provider", "* required",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Optional fields carry no marker.
	if strings.Contains(out, "* terraformVersion") {
		t.Errorf("an optional field is marked required:\n%s", out)
	}
}

func TestBackstageOfertasAcceptsASinglePageObject(t *testing.T) {
	// A one-page template may store the object directly instead of an array.
	fake := &fakeBackstage{entities: []backstage.Entity{{
		Kind:     "Template",
		Metadata: backstage.EntityMetadata{Name: "simple", Namespace: "default"},
		Spec: map[string]interface{}{"parameters": map[string]interface{}{
			"properties": map[string]interface{}{
				"repoUrl": map[string]interface{}{"title": "Repository"},
			},
			"required": []interface{}{"repoUrl"},
		}},
	}}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "ofertas", "--json")
	if err != nil {
		t.Fatalf("backstage ofertas: %v", err)
	}
	if !strings.Contains(out, `"repoUrl"`) || !strings.Contains(out, `"required": true`) {
		t.Errorf("single-page parameters were not read:\n%s", out)
	}
}

func TestBackstageOfertasWithNoParameters(t *testing.T) {
	fake := &fakeBackstage{entities: []backstage.Entity{{
		Kind:     "Template",
		Metadata: backstage.EntityMetadata{Name: "bare", Namespace: "default"},
	}}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "ofertas", "--json")
	if err != nil {
		t.Fatalf("backstage ofertas --json: %v", err)
	}
	// Never null: a consumer must be able to iterate fields unconditionally.
	if !strings.Contains(out, `"fields": []`) {
		t.Errorf("fields is not an empty array:\n%s", out)
	}

	out, err = runRoot(t, backstageDeps(fake), "backstage", "ofertas")
	if err != nil {
		t.Fatalf("backstage ofertas: %v", err)
	}
	if !strings.Contains(out, "asks for nothing") {
		t.Errorf("output:\n%s", out)
	}
}

func TestBackstageOfertasEmptyCatalog(t *testing.T) {
	out, err := runRoot(t, backstageDeps(&fakeBackstage{}), "backstage", "ofertas")
	if err != nil {
		t.Fatalf("backstage ofertas: %v", err)
	}
	if !strings.Contains(out, "No templates") {
		t.Errorf("output:\n%s", out)
	}

	out, err = runRoot(t, backstageDeps(&fakeBackstage{}), "backstage", "ofertas", "--json")
	if err != nil {
		t.Fatalf("backstage ofertas --json: %v", err)
	}
	if !strings.Contains(out, `"templates": []`) {
		t.Errorf("empty listing is not an empty array:\n%s", out)
	}
}

func TestBackstageOfertasIgnoresAMalformedSchema(t *testing.T) {
	fake := &fakeBackstage{entities: []backstage.Entity{{
		Kind:     "Template",
		Metadata: backstage.EntityMetadata{Name: "odd", Namespace: "default"},
		Spec: map[string]interface{}{"parameters": []interface{}{
			"not an object",
			map[string]interface{}{"properties": "not an object"},
			map[string]interface{}{
				"properties": map[string]interface{}{"ok": "not an object"},
				"required":   "not a list",
			},
		}},
	}}}

	out, err := runRoot(t, backstageDeps(fake), "backstage", "ofertas", "--json")
	if err != nil {
		t.Fatalf("backstage ofertas: %v", err)
	}
	// The one readable property survives, with an empty description, and
	// nothing panics on the rest.
	if !strings.Contains(out, `"name": "ok"`) || !strings.Contains(out, `"required": false`) {
		t.Errorf("output:\n%s", out)
	}
}

func TestBackstageOfertasThroughVault(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(kvSecret("secret/backstage", map[string]string{
		"url": "https://acme.example.com", "token": "vault-issued",
	}), &got)

	if _, err := runRoot(t, deps, "backstage", "ofertas", "--vault-secret", "secret/backstage"); err != nil {
		t.Fatalf("backstage ofertas --vault-secret: %v", err)
	}
	if got.BaseURL != "https://acme.example.com" || got.Token != "vault-issued" {
		t.Fatalf("config = %+v", got)
	}
}

// ---------------------------------------------------------------------------
// backstage create
// ---------------------------------------------------------------------------

// typedTemplate declares a parameter of each type create has to convert.
func typedTemplate() backstage.Entity {
	return backstage.Entity{
		Kind:     "Template",
		Metadata: backstage.EntityMetadata{Name: "svc", Namespace: "default"},
		Spec: map[string]interface{}{
			"parameters": []interface{}{map[string]interface{}{
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string", "title": "Name"},
					"port":        map[string]interface{}{"type": "integer", "title": "Port"},
					"ratio":       map[string]interface{}{"type": "number", "title": "Ratio"},
					"includeAdr":  map[string]interface{}{"type": "boolean", "title": "ADR"},
					"tags":        map[string]interface{}{"type": "array", "title": "Tags"},
					"untypedNote": map[string]interface{}{"title": "Note"},
				},
				"required": []interface{}{"name"},
			}},
		},
	}
}

func createDeps(t *testing.T, template backstage.Entity, fake *fakeBackstage) Deps {
	t.Helper()
	fake.byRef = map[string]backstage.Entity{template.Ref().String(): template}
	return backstageDeps(fake)
}

func TestBackstageCreateSubmitsCoercedValues(t *testing.T) {
	fake := &fakeBackstage{statuses: []string{"completed"}}
	deps := createDeps(t, typedTemplate(), fake)

	_, err := runRoot(t, deps, "backstage", "create", "svc",
		"--field", "name=payments",
		"--field", "port=8080",
		"--field", "ratio=0.5",
		"--field", "includeAdr=true",
		"--field", "untypedNote=whatever")
	if err != nil {
		t.Fatalf("backstage create: %v", err)
	}

	if len(fake.scaffolded) != 1 {
		t.Fatalf("submitted %d runs, want 1", len(fake.scaffolded))
	}
	call := fake.scaffolded[0]
	if call.Ref.String() != "template:default/svc" {
		t.Errorf("ref = %s", call.Ref)
	}
	// Each value arrives as the type the schema declares, not as a string.
	want := map[string]interface{}{
		"name": "payments", "port": int64(8080), "ratio": 0.5,
		"includeAdr": true, "untypedNote": "whatever",
	}
	for name, expected := range want {
		if got := call.Values[name]; got != expected {
			t.Errorf("values[%q] = %#v, want %#v", name, got, expected)
		}
	}
}

func TestBackstageCreateRejectsAnUnknownField(t *testing.T) {
	fake := &fakeBackstage{}
	deps := createDeps(t, typedTemplate(), fake)

	_, err := runRoot(t, deps, "backstage", "create", "svc",
		"--field", "name=x", "--field", "prot=8080")
	if err == nil {
		t.Fatal("want an error for a misspelled parameter")
	}
	if !strings.Contains(err.Error(), `"prot"`) || !strings.Contains(err.Error(), "port") {
		t.Errorf("error does not name the typo and the real parameters: %v", err)
	}
	if len(fake.scaffolded) != 0 {
		t.Error("the run was submitted despite the bad field")
	}
}

func TestBackstageCreateRejectsAMissingRequiredField(t *testing.T) {
	fake := &fakeBackstage{}
	deps := createDeps(t, typedTemplate(), fake)

	_, err := runRoot(t, deps, "backstage", "create", "svc", "--field", "port=1")
	if err == nil || !strings.Contains(err.Error(), `"name"`) {
		t.Fatalf("error = %v, want it to name the missing field", err)
	}
	if len(fake.scaffolded) != 0 {
		t.Error("the run was submitted with a required field missing")
	}
}

func TestBackstageCreateRejectsBadTypesAndShapes(t *testing.T) {
	cases := []struct {
		name  string
		field string
		want  string
	}{
		{"boolean", "includeAdr=perhaps", "boolean"},
		{"integer", "port=eighty", "integer"},
		{"number", "ratio=half", "number"},
		{"array", "tags=a,b", "array"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeBackstage{}
			deps := createDeps(t, typedTemplate(), fake)

			_, err := runRoot(t, deps, "backstage", "create", "svc",
				"--field", "name=x", "--field", tc.field)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if len(fake.scaffolded) != 0 {
				t.Error("the run was submitted anyway")
			}
		})
	}
}

func TestBackstageCreateRejectsMalformedFieldFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--field", "novalue"},
		{"--field", "=orphan"},
		{"--field", "name=a", "--field", "name=b"},
	} {
		fake := &fakeBackstage{}
		deps := createDeps(t, typedTemplate(), fake)

		full := append([]string{"backstage", "create", "svc"}, args...)
		if _, err := runRoot(t, deps, full...); err == nil {
			t.Errorf("%v: want an error", args)
		}
		if len(fake.scaffolded) != 0 {
			t.Errorf("%v: the run was submitted", args)
		}
	}
}

func TestBackstageCreateDryRunSendsNothing(t *testing.T) {
	fake := &fakeBackstage{}
	deps := createDeps(t, typedTemplate(), fake)

	out, err := runRoot(t, deps, "backstage", "create", "svc",
		"--field", "name=payments", "--field", "port=8080", "--dry-run")
	if err != nil {
		t.Fatalf("backstage create --dry-run: %v", err)
	}
	if len(fake.scaffolded) != 0 {
		t.Fatal("--dry-run submitted a run")
	}
	for _, want := range []string{"template:default/svc", "payments", "8080", "Nothing was sent"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBackstageCreateDryRunValidatesToo(t *testing.T) {
	fake := &fakeBackstage{}
	deps := createDeps(t, typedTemplate(), fake)

	if _, err := runRoot(t, deps, "backstage", "create", "svc", "--dry-run"); err == nil {
		t.Fatal("--dry-run accepted a missing required field")
	}
}

func TestBackstageCreateNoWait(t *testing.T) {
	fake := &fakeBackstage{}
	deps := createDeps(t, typedTemplate(), fake)

	out, err := runRoot(t, deps, "backstage", "create", "svc",
		"--field", "name=x", "--no-wait")
	if err != nil {
		t.Fatalf("backstage create --no-wait: %v", err)
	}
	if !strings.Contains(out, "task-1") || !strings.Contains(out, "/create/tasks/task-1") {
		t.Errorf("output:\n%s", out)
	}
	// Nothing was followed.
	if fake.taskCalls != 0 {
		t.Errorf("--no-wait polled the task %d times", fake.taskCalls)
	}
}

func TestBackstageCreateFollowsTheRun(t *testing.T) {
	fake := &fakeBackstage{
		statuses: []string{"processing", "completed"},
		events: []backstage.TaskEvent{
			{ID: 1, Type: "log", Message: "Beginning step Fetch"},
			{ID: 2, Type: "log", Message: "Beginning step Publish"},
			{ID: 3, Type: "completion", Message: "Run completed with status: completed",
				Output: map[string]interface{}{"links": []interface{}{
					map[string]interface{}{"title": "Repository", "url": "https://github.com/acme/payments"},
					map[string]interface{}{"title": "Open in catalog", "entityRef": "component:default/payments"},
				}}},
		},
	}
	deps := createDeps(t, typedTemplate(), fake)

	out, err := runRoot(t, deps, "backstage", "create", "svc", "--field", "name=payments")
	if err != nil {
		t.Fatalf("backstage create: %v", err)
	}
	for _, want := range []string{
		"Beginning step Fetch", "Beginning step Publish", "completed",
		"Repository", "https://github.com/acme/payments", "component:default/payments",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBackstageCreateFailedRunExitsNonZero(t *testing.T) {
	fake := &fakeBackstage{
		statuses: []string{"failed"},
		events: []backstage.TaskEvent{
			{ID: 1, Type: "log", Message: "Beginning step Publish"},
			{ID: 2, Type: "completion", Message: "Run completed with status: failed",
				Error: "InputError: No token available for host: github.com"},
		},
	}
	deps := createDeps(t, typedTemplate(), fake)

	out, err := runRoot(t, deps, "backstage", "create", "svc", "--field", "name=x")
	if err == nil {
		t.Fatal("a failed run exited zero")
	}
	// The log is already on screen, so the entrypoint must not print again.
	var silent SilentError
	if !errors.As(err, &silent) {
		t.Errorf("error is not a SilentError: %v", err)
	}
	if !strings.Contains(out, "No token available") {
		t.Errorf("the failure reason was not shown:\n%s", out)
	}
}

func TestBackstageCreateJSON(t *testing.T) {
	fake := &fakeBackstage{
		statuses: []string{"completed"},
		events: []backstage.TaskEvent{
			{ID: 1, Type: "log", Message: "Beginning step Fetch"},
			{ID: 2, Type: "completion", Output: map[string]interface{}{"remoteUrl": "https://github.com/acme/x"}},
		},
	}
	deps := createDeps(t, typedTemplate(), fake)

	out, err := runRoot(t, deps, "backstage", "create", "svc", "--field", "name=x", "--json")
	if err != nil {
		t.Fatalf("backstage create --json: %v", err)
	}

	var payload struct {
		ID     string                 `json:"id"`
		Status string                 `json:"status"`
		URL    string                 `json:"url"`
		Log    []string               `json:"log"`
		Output map[string]interface{} `json:"output"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decoding %q: %v", out, err)
	}
	if payload.ID != "task-1" || payload.Status != "completed" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(payload.Log) != 1 || payload.Log[0] != "Beginning step Fetch" {
		t.Errorf("log = %v", payload.Log)
	}
	if payload.Output["remoteUrl"] != "https://github.com/acme/x" {
		t.Errorf("output = %v", payload.Output)
	}
}

func TestBackstageCreateUnknownTemplate(t *testing.T) {
	fake := &fakeBackstage{}
	deps := backstageDeps(fake)

	_, err := runRoot(t, deps, "backstage", "create", "nope", "--field", "name=x")
	if !errors.Is(err, backstage.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if len(fake.scaffolded) != 0 {
		t.Error("a run was submitted for a template that does not exist")
	}
}

func TestBackstageCreateDefaultsTheKindToTemplate(t *testing.T) {
	fake := &fakeBackstage{statuses: []string{"completed"}}
	deps := createDeps(t, typedTemplate(), fake)

	if _, err := runRoot(t, deps, "backstage", "create", "svc", "--field", "name=x"); err != nil {
		t.Fatalf("backstage create: %v", err)
	}
	if len(fake.refs) != 1 || fake.refs[0].String() != "template:default/svc" {
		t.Fatalf("refs = %v", fake.refs)
	}
}

func TestBackstageCreateTimesOutOnAStuckRun(t *testing.T) {
	fake := &fakeBackstage{statuses: []string{"processing"}}
	deps := createDeps(t, typedTemplate(), fake)

	_, err := runRoot(t, deps, "backstage", "create", "svc",
		"--field", "name=x", "--timeout", "1ms")
	if err == nil || !strings.Contains(err.Error(), "still processing") {
		t.Fatalf("error = %v, want the timeout to be reported", err)
	}
	if !strings.Contains(err.Error(), "/create/tasks/task-1") {
		t.Errorf("the timeout does not say where to follow the run: %v", err)
	}
}
