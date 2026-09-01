package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/backstage"
	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
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
