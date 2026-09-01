package backstage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// These tests drive the client against a stand-in Backstage over HTTP, so the
// URL building and the JSON decoding are actually exercised rather than
// stubbed at the interface.

type recordedRequest struct {
	method string
	path   string
	query  url.Values
	auth   string
	accept string
}

// standIn is a Backstage catalog whose responses each test sets up.
type standIn struct {
	t        *testing.T
	server   *httptest.Server
	requests []recordedRequest

	// pages are returned by by-query in order, one per request.
	pages []string
	// entity is returned by by-name; empty means 404.
	entity string
	// status, when non-zero, is returned instead of any body.
	status int
	// errorBody is the body sent alongside status.
	errorBody string
}

func newStandIn(t *testing.T) *standIn {
	t.Helper()

	s := &standIn{t: t}
	mux := http.NewServeMux()

	record := func(r *http.Request) {
		s.requests = append(s.requests, recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			auth:   r.Header.Get("Authorization"),
			accept: r.Header.Get("Accept"),
		})
	}

	fail := func(w http.ResponseWriter) bool {
		if s.status == 0 {
			return false
		}
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.errorBody))
		return true
	}

	mux.HandleFunc("/api/catalog/entities/by-query", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if fail(w) {
			return
		}
		body := `{"items":[],"totalItems":0,"pageInfo":{}}`
		if len(s.pages) > 0 {
			body, s.pages = s.pages[0], s.pages[1:]
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	mux.HandleFunc("/api/catalog/entity-facets", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if fail(w) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"facets":{"kind":[{"value":"Component","count":42},{"value":"API","count":7}]}}`))
	})

	mux.HandleFunc("/api/catalog/entities/by-name/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if fail(w) {
			return
		}
		if s.entity == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"name":"NotFoundError","message":"No entity"},"response":{"statusCode":404}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s.entity))
	})

	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

// client returns a client pointed at the stand-in, with the environment
// cleared so a developer's own BACKSTAGE_* variables cannot reach the test.
func (s *standIn) client(token string) Client {
	s.t.Helper()
	s.t.Setenv("BACKSTAGE_BASE_URL", "")
	s.t.Setenv("BACKSTAGE_URL", "")
	s.t.Setenv("BACKSTAGE_TOKEN", "")

	c, err := New(Config{
		BaseURL:    s.server.URL,
		Token:      token,
		HTTPClient: s.server.Client(),
	})
	if err != nil {
		s.t.Fatalf("New: %v", err)
	}
	return c
}

func (s *standIn) last() recordedRequest {
	s.t.Helper()
	if len(s.requests) == 0 {
		s.t.Fatal("no request was made")
	}
	return s.requests[len(s.requests)-1]
}

// ---------------------------------------------------------------------------

func TestNewNormalizesTheBaseURL(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  string
		error bool
	}{
		{name: "as given", in: "https://backstage.example.com", want: "https://backstage.example.com"},
		{name: "trailing slash", in: "https://backstage.example.com/", want: "https://backstage.example.com"},
		{name: "bare host defaults to TLS", in: "backstage.example.com", want: "https://backstage.example.com"},
		{name: "plaintext is kept when asked for", in: "http://localhost:7007", want: "http://localhost:7007"},
		{name: "catalog suffix is dropped", in: "https://backstage.example.com/api/catalog", want: "https://backstage.example.com"},
		{name: "sub-path is kept", in: "https://example.com/backstage", want: "https://example.com/backstage"},
		{name: "query is dropped", in: "https://example.com/?a=b", want: "https://example.com"},
		{name: "wrong scheme", in: "ftp://example.com", error: true},
		{name: "no host", in: "https://", error: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BACKSTAGE_BASE_URL", "")
			t.Setenv("BACKSTAGE_URL", "")

			c, err := New(Config{BaseURL: tc.in})
			if tc.error {
				if err == nil {
					t.Fatalf("New(%q) = %q, want an error", tc.in, c.BaseURL())
				}
				return
			}
			if err != nil {
				t.Fatalf("New(%q): %v", tc.in, err)
			}
			if got := c.BaseURL(); got != tc.want {
				t.Errorf("BaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewWithoutAnAddress(t *testing.T) {
	t.Setenv("BACKSTAGE_BASE_URL", "")
	t.Setenv("BACKSTAGE_URL", "")

	if _, err := New(Config{}); !errors.Is(err, ErrNoBaseURL) {
		t.Fatalf("New: %v, want ErrNoBaseURL", err)
	}
}

func TestNewReadsTheEnvironment(t *testing.T) {
	t.Setenv("BACKSTAGE_BASE_URL", "")
	t.Setenv("BACKSTAGE_URL", "https://from-env.example.com")

	c, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.BaseURL(); got != "https://from-env.example.com" {
		t.Errorf("BaseURL() = %q", got)
	}
}

func TestParseEntityRef(t *testing.T) {
	cases := []struct {
		in          string
		defaultKind string
		want        string
		error       bool
	}{
		{in: "component:default/my-service", want: "component:default/my-service"},
		{in: "component:my-service", want: "component:default/my-service"},
		{in: "my-service", defaultKind: "component", want: "component:default/my-service"},
		{in: "Component:Default/My-Service", want: "component:default/my-service"},
		{in: "group:platform/team-a", want: "group:platform/team-a"},
		{in: "  component:my-service  ", want: "component:default/my-service"},
		{in: "my-service", error: true},
		{in: "", error: true},
		{in: ":my-service", error: true},
		{in: "component:/my-service", error: true},
		{in: "component:default/", error: true},
		{in: "component:a/b/c", error: true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			ref, err := ParseEntityRef(tc.in, tc.defaultKind)
			if tc.error {
				if err == nil {
					t.Fatalf("ParseEntityRef(%q) = %s, want an error", tc.in, ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEntityRef(%q): %v", tc.in, err)
			}
			if got := ref.String(); got != tc.want {
				t.Errorf("ParseEntityRef(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEntitiesEncodesTheQuery(t *testing.T) {
	s := newStandIn(t)

	_, err := s.client("s3cr3t").Entities(context.Background(), EntityQuery{
		Filters: []Filter{
			Filter{}.Add("kind", "component").Add("spec.type", "service", "website"),
			Filter{}.Add("kind", "api"),
			Filter{}.Add("metadata.annotations.backstage.io/source-location"),
		},
		Fields:         []string{"kind", "metadata.name"},
		Limit:          25,
		OrderBy:        []Order{{Field: "metadata.name"}, {Field: "spec.type", Descending: true}},
		FullTextTerm:   "payments",
		FullTextFields: []string{"metadata.name"},
	})
	if err != nil {
		t.Fatalf("Entities: %v", err)
	}

	got := s.last()
	if got.method != http.MethodGet || got.path != "/api/catalog/entities/by-query" {
		t.Errorf("%s %s, want GET /api/catalog/entities/by-query", got.method, got.path)
	}
	if got.auth != "Bearer s3cr3t" {
		t.Errorf("Authorization header = %q", got.auth)
	}
	if got.accept != "application/json" {
		t.Errorf("Accept header = %q", got.accept)
	}

	// Keys within a filter are sorted, so the encoding is deterministic, and
	// a key with several values repeats the key.
	wantFilters := []string{
		"kind=component,spec.type=service,spec.type=website",
		"kind=api",
		"metadata.annotations.backstage.io/source-location",
	}
	if filters := got.query["filter"]; !equal(filters, wantFilters) {
		t.Errorf("filter = %q, want %q", filters, wantFilters)
	}
	if fields := got.query["fields"]; !equal(fields, []string{"kind", "metadata.name"}) {
		t.Errorf("fields = %q", fields)
	}
	if order := got.query["orderField"]; !equal(order, []string{"metadata.name,asc", "spec.type,desc"}) {
		t.Errorf("orderField = %q", order)
	}
	if got.query.Get("limit") != "25" {
		t.Errorf("limit = %q", got.query.Get("limit"))
	}
	if got.query.Get("fullTextFilterTerm") != "payments" {
		t.Errorf("fullTextFilterTerm = %q", got.query.Get("fullTextFilterTerm"))
	}
}

func TestEntitiesDefaultsTheLimit(t *testing.T) {
	s := newStandIn(t)

	if _, err := s.client("").Entities(context.Background(), EntityQuery{}); err != nil {
		t.Fatalf("Entities: %v", err)
	}

	if got := s.last().query.Get("limit"); got != itoa(DefaultPageSize) {
		t.Errorf("limit = %q, want %d", got, DefaultPageSize)
	}
	if auth := s.last().auth; auth != "" {
		t.Errorf("Authorization header = %q, want none when no token is configured", auth)
	}
}

func TestEntitiesDecodesAPage(t *testing.T) {
	s := newStandIn(t)
	s.pages = []string{`{
		"items": [{
			"apiVersion": "backstage.io/v1alpha1",
			"kind": "Component",
			"metadata": {
				"name": "payments",
				"namespace": "default",
				"title": "Payments API",
				"tags": ["go", "tier-1"],
				"annotations": {"backstage.io/source-location": "url:https://github.com/acme/payments"}
			},
			"spec": {"type": "service", "lifecycle": "production", "owner": "group:default/payments-team"},
			"relations": [
				{"type": "ownedBy", "targetRef": "group:default/payments-team"},
				{"type": "dependsOn", "targetRef": "resource:default/payments-db"},
				{"type": "dependsOn", "targetRef": "component:default/ledger"}
			]
		}],
		"totalItems": 1,
		"pageInfo": {"nextCursor": "abc"}
	}`}

	page, err := s.client("").Entities(context.Background(), EntityQuery{})
	if err != nil {
		t.Fatalf("Entities: %v", err)
	}

	if len(page.Items) != 1 || page.TotalItems != 1 || page.NextCursor != "abc" {
		t.Fatalf("page = %+v", page)
	}

	entity := page.Items[0]
	if got := entity.Ref().String(); got != "component:default/payments" {
		t.Errorf("Ref() = %q", got)
	}
	if got := entity.DisplayName(); got != "Payments API" {
		t.Errorf("DisplayName() = %q", got)
	}
	if entity.Type() != "service" || entity.Lifecycle() != "production" {
		t.Errorf("Type()/Lifecycle() = %q/%q", entity.Type(), entity.Lifecycle())
	}
	if got := entity.Annotation("backstage.io/source-location"); got != "url:https://github.com/acme/payments" {
		t.Errorf("Annotation() = %q", got)
	}
	want := []string{"resource:default/payments-db", "component:default/ledger"}
	if got := entity.Targets("dependsOn"); !equal(got, want) {
		t.Errorf("Targets(dependsOn) = %q, want %q", got, want)
	}
	if got := entity.Targets("partOf"); got != nil {
		t.Errorf("Targets(partOf) = %q, want nil", got)
	}
}

func TestEntityRefFillsInTheNamespace(t *testing.T) {
	entity := Entity{Kind: "Component", Metadata: EntityMetadata{Name: "x"}}
	if got := entity.Ref().String(); got != "component:default/x" {
		t.Errorf("Ref() = %q", got)
	}
}

func TestSpecAccessorsTolerateMissingAndWrongTypes(t *testing.T) {
	entity := Entity{Spec: map[string]interface{}{"type": 42}}
	if got := entity.Type(); got != "" {
		t.Errorf("Type() = %q, want empty for a non-string spec.type", got)
	}
	if got := (Entity{}).Owner(); got != "" {
		t.Errorf("Owner() = %q, want empty for a nil spec", got)
	}
}

func TestAllEntitiesFollowsTheCursor(t *testing.T) {
	s := newStandIn(t)
	s.pages = []string{
		`{"items":[{"kind":"Component","metadata":{"name":"a"}}],"pageInfo":{"nextCursor":"page-2"}}`,
		`{"items":[{"kind":"Component","metadata":{"name":"b"}}],"pageInfo":{"nextCursor":"page-3"}}`,
		`{"items":[{"kind":"Component","metadata":{"name":"c"}}],"pageInfo":{}}`,
	}

	entities, err := s.client("").AllEntities(context.Background(), EntityQuery{
		Filters: []Filter{Filter{}.Add("kind", "component")},
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("AllEntities: %v", err)
	}

	var names []string
	for _, e := range entities {
		names = append(names, e.Metadata.Name)
	}
	if !equal(names, []string{"a", "b", "c"}) {
		t.Fatalf("names = %q", names)
	}
	if len(s.requests) != 3 {
		t.Fatalf("made %d requests, want 3", len(s.requests))
	}

	// The first request carries the filter; the rest carry only the cursor,
	// because the cursor already encodes it.
	if got := s.requests[0].query.Get("filter"); got != "kind=component" {
		t.Errorf("first filter = %q", got)
	}
	for i, want := range []string{"page-2", "page-3"} {
		got := s.requests[i+1]
		if got.query.Get("cursor") != want {
			t.Errorf("request %d cursor = %q, want %q", i+1, got.query.Get("cursor"), want)
		}
		if got.query.Has("filter") {
			t.Errorf("request %d resent the filter", i+1)
		}
		if got.query.Get("limit") != "1" {
			t.Errorf("request %d limit = %q, want the original 1", i+1, got.query.Get("limit"))
		}
	}
}

func TestEntityByName(t *testing.T) {
	s := newStandIn(t)
	s.entity = `{"kind":"Component","metadata":{"name":"payments","namespace":"default"},"spec":{"owner":"team-a"}}`

	ref, err := ParseEntityRef("component:payments", "")
	if err != nil {
		t.Fatalf("ParseEntityRef: %v", err)
	}

	entity, err := s.client("").Entity(context.Background(), ref)
	if err != nil {
		t.Fatalf("Entity: %v", err)
	}
	if entity.Owner() != "team-a" {
		t.Errorf("Owner() = %q", entity.Owner())
	}
	if got := s.last().path; got != "/api/catalog/entities/by-name/component/default/payments" {
		t.Errorf("path = %q", got)
	}
}

func TestEntityNotFound(t *testing.T) {
	s := newStandIn(t)
	s.entity = "" // the stand-in answers 404

	_, err := s.client("").Entity(context.Background(), EntityRef{Kind: "component", Namespace: "default", Name: "ghost"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Entity: %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "component:default/ghost") {
		t.Errorf("error %q does not name the entity", err)
	}
}

func TestEntityRejectsAnIncompleteRef(t *testing.T) {
	s := newStandIn(t)

	if _, err := s.client("").Entity(context.Background(), EntityRef{Name: "payments"}); err == nil {
		t.Fatal("Entity with no kind: want an error")
	}
	if len(s.requests) != 0 {
		t.Errorf("made %d requests, want none: the ref is invalid before any I/O", len(s.requests))
	}
}

func TestAPIErrorReadsTheCatalogEnvelope(t *testing.T) {
	s := newStandIn(t)
	s.status = http.StatusBadRequest
	s.errorBody = `{"error":{"name":"InputError","message":"invalid filter"},"response":{"statusCode":400}}`

	_, err := s.client("").Entities(context.Background(), EntityQuery{})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Entities: %v, want an *APIError", err)
	}
	if apiErr.StatusCode != 400 || apiErr.Name != "InputError" || apiErr.Message != "invalid filter" {
		t.Fatalf("APIError = %+v", apiErr)
	}
	if !strings.Contains(err.Error(), "invalid filter") {
		t.Errorf("error = %q", err)
	}
}

func TestAPIErrorOnAnUnauthenticatedRequest(t *testing.T) {
	s := newStandIn(t)
	s.status = http.StatusUnauthorized
	s.errorBody = "<html><body>Unauthorized</body></html>\nsecond line"

	err := s.client("wrong").Ping(context.Background())
	if err == nil {
		t.Fatal("Ping: want an error")
	}
	if !strings.Contains(err.Error(), "BACKSTAGE_TOKEN") {
		t.Errorf("error %q does not point at the token", err)
	}
	if strings.Contains(err.Error(), "second line") {
		t.Errorf("error %q kept more than one line of the body", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Errorf("error %q leaked the token", err)
	}
}

func TestFacets(t *testing.T) {
	s := newStandIn(t)

	facets, err := s.client("").Facets(context.Background(), "kind", "spec.type")
	if err != nil {
		t.Fatalf("Facets: %v", err)
	}
	if got := facets["kind"]; len(got) != 2 || got[0].Value != "Component" || got[0].Count != 42 {
		t.Fatalf("facets = %+v", facets)
	}
	if got := s.last().query["facet"]; !equal(got, []string{"kind", "spec.type"}) {
		t.Errorf("facet = %q", got)
	}
}

func TestFacetsWithoutAField(t *testing.T) {
	s := newStandIn(t)

	if _, err := s.client("").Facets(context.Background()); err == nil {
		t.Fatal("Facets with no field: want an error")
	}
	if len(s.requests) != 0 {
		t.Errorf("made %d requests, want none", len(s.requests))
	}
}

func TestPing(t *testing.T) {
	s := newStandIn(t)

	if err := s.client("t").Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got := s.last().path; got != "/api/catalog/entity-facets" {
		t.Errorf("path = %q", got)
	}
}

func TestRequestsHonourTheContext(t *testing.T) {
	s := newStandIn(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.client("").Entities(ctx, EntityQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Entities: %v, want context.Canceled", err)
	}
}

func TestDecodingAMalformedBody(t *testing.T) {
	s := newStandIn(t)
	s.pages = []string{"not json"}

	if _, err := s.client("").Entities(context.Background(), EntityQuery{}); err == nil {
		t.Fatal("Entities: want a decoding error")
	}
}

// equal compares two string slices positionally.
func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
