package backstage

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// DefaultNamespace is the namespace Backstage assumes when a reference omits
// one.
const DefaultNamespace = "default"

// Entity is one catalog entry.
//
// Spec is left untyped on purpose: its shape depends on Kind -- a Component
// has type/lifecycle/owner, a User has profile/memberOf, a custom kind has
// whatever its schema says -- and modelling every kind here would mean this
// package rejecting entities it does not recognise. The accessors below cover
// the fields shared by the built-in kinds; anything else is read from the map.
type Entity struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   EntityMetadata         `json:"metadata"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
	Relations  []Relation             `json:"relations,omitempty"`
}

// EntityMetadata is the part of an entity that every kind shares.
type EntityMetadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	UID         string            `json:"uid,omitempty"`
	Etag        string            `json:"etag,omitempty"`
}

// Relation is an edge to another entity, e.g. ownedBy -> group:default/team-a.
type Relation struct {
	Type      string `json:"type"`
	TargetRef string `json:"targetRef"`
}

// Ref returns the entity's own reference, filling in the default namespace.
func (e Entity) Ref() EntityRef {
	return EntityRef{
		Kind:      e.Kind,
		Namespace: firstNonEmpty(e.Metadata.Namespace, DefaultNamespace),
		Name:      e.Metadata.Name,
	}
}

// DisplayName is the title if the entity has one, otherwise the name. It is
// what a human should see; Ref is what a machine should use.
func (e Entity) DisplayName() string {
	return firstNonEmpty(e.Metadata.Title, e.Metadata.Name)
}

// SpecString reads a string field out of the spec, returning "" when the field
// is absent or is not a string. Owner-shaped fields can arrive as a plain name
// ("team-a") or as a full reference ("group:default/team-a"); this returns
// whatever the catalog stored, without normalising.
func (e Entity) SpecString(field string) string {
	value, _ := e.Spec[field].(string)
	return value
}

// Type is spec.type: "service", "website", "library" for a Component, and the
// equivalent discriminator on other kinds.
func (e Entity) Type() string { return e.SpecString("type") }

// Lifecycle is spec.lifecycle: "production", "experimental", "deprecated".
func (e Entity) Lifecycle() string { return e.SpecString("lifecycle") }

// Owner is spec.owner.
func (e Entity) Owner() string { return e.SpecString("owner") }

// Annotation reads one metadata annotation, e.g. "backstage.io/source-location".
func (e Entity) Annotation(key string) string { return e.Metadata.Annotations[key] }

// Targets returns the target references of every relation of a given type,
// e.g. Targets("ownedBy") or Targets("dependsOn").
func (e Entity) Targets(relationType string) []string {
	var targets []string
	for _, relation := range e.Relations {
		if relation.Type == relationType {
			targets = append(targets, relation.TargetRef)
		}
	}
	return targets
}

// EntityPage is one page of a paginated listing.
type EntityPage struct {
	Items      []Entity
	TotalItems int
	// NextCursor is empty on the last page. Pass it back as EntityQuery.Cursor
	// to continue, or use AllEntities to have that done for you.
	NextCursor string
	PrevCursor string
}

// ---------------------------------------------------------------------------
// References
// ---------------------------------------------------------------------------

// EntityRef identifies an entity. Its string form is Backstage's canonical
// "<kind>:<namespace>/<name>", lowercased -- the catalog compares references
// case-insensitively and stores them folded.
type EntityRef struct {
	Kind      string
	Namespace string
	Name      string
}

// String renders the canonical reference.
func (r EntityRef) String() string {
	return strings.ToLower(r.Kind + ":" + firstNonEmpty(r.Namespace, DefaultNamespace) + "/" + r.Name)
}

func (r EntityRef) validate() error {
	switch {
	case r.Kind == "":
		return fmt.Errorf("entity reference %q has no kind", r.Name)
	case r.Name == "":
		return fmt.Errorf("entity reference has no name")
	}
	return nil
}

// ParseEntityRef accepts the forms a user or a relation will produce:
//
//	component:default/my-service   full
//	component:my-service           kind and name, default namespace
//	my-service                     name only, needs defaultKind
//
// defaultKind fills in the kind when the reference omits it, which is how a
// command with a --kind flag or a kind-specific subcommand supplies context.
// Pass "" to require the caller to be explicit.
func ParseEntityRef(raw, defaultKind string) (EntityRef, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return EntityRef{}, fmt.Errorf("empty entity reference")
	}

	ref := EntityRef{Kind: defaultKind, Namespace: DefaultNamespace}

	if kind, rest, found := strings.Cut(text, ":"); found {
		if kind == "" {
			return EntityRef{}, fmt.Errorf("entity reference %q has an empty kind", raw)
		}
		ref.Kind, text = kind, rest
	}

	if namespace, name, found := strings.Cut(text, "/"); found {
		if namespace == "" || name == "" {
			return EntityRef{}, fmt.Errorf("entity reference %q has an empty namespace or name", raw)
		}
		ref.Namespace, text = namespace, name
	}

	if strings.ContainsAny(text, ":/") {
		return EntityRef{}, fmt.Errorf("entity reference %q has too many parts, want [<kind>:][<namespace>/]<name>", raw)
	}
	ref.Name = text

	if ref.Kind == "" {
		return EntityRef{}, fmt.Errorf("entity reference %q has no kind, write it as component:%s", raw, ref.Name)
	}
	if ref.Name == "" {
		return EntityRef{}, fmt.Errorf("entity reference %q has no name", raw)
	}
	return ref, nil
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

// Filter is one filter clause. Every key must match, and a key listed with
// several values matches any of them -- AND across keys, OR within a key.
// A key with no values matches entities where that field merely exists, which
// is how you find "everything carrying this annotation".
//
// Keys are dotted field paths into the entity as stored:
//
//	kind
//	spec.type
//	spec.lifecycle
//	metadata.namespace
//	metadata.tags
//	metadata.annotations.backstage.io/source-location
//	relations.ownedBy
type Filter map[string][]string

// Add appends values to a key and returns the filter, for chaining.
func (f Filter) Add(key string, values ...string) Filter {
	f[key] = append(f[key], values...)
	return f
}

// encode renders the filter the way the catalog's filter parameter expects:
// comma-separated key=value pairs. Keys are sorted so the same filter always
// produces the same URL, which keeps tests and HTTP caches honest.
func (f Filter) encode() string {
	keys := make([]string, 0, len(f))
	for key := range f {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		if len(f[key]) == 0 {
			parts = append(parts, key)
			continue
		}
		for _, value := range f[key] {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, ",")
}

// Order is one sort key.
type Order struct {
	Field      string
	Descending bool
}

// EntityQuery describes a listing. The zero value lists everything, one
// DefaultPageSize page at a time.
type EntityQuery struct {
	// Filters are OR'd against each other: an entity matching any one clause
	// is returned. Use a single Filter with several keys for AND.
	Filters []Filter

	// Fields trims the response to these dotted paths, e.g.
	// []string{"kind", "metadata.name", "spec.owner"}. Worth setting when
	// listing a large catalog: the entities themselves are the bulk of the
	// payload. Empty returns whole entities.
	Fields []string

	// Limit is the page size. Zero means DefaultPageSize. The server may
	// return fewer.
	Limit int

	// Cursor continues a previous page. When set, the filters are already
	// encoded inside it and every other field except Limit is ignored.
	Cursor string

	// OrderBy sorts the result. Backstage requires a stable order for cursor
	// pagination and applies its own default when this is empty.
	OrderBy []Order

	// FullTextTerm searches across the entity, the way the catalog's search
	// box does. It is applied on top of Filters.
	FullTextTerm string

	// FullTextFields restricts FullTextTerm to these dotted paths.
	FullTextFields []string
}

// values renders the query as the by-query endpoint's parameters.
func (q EntityQuery) values() url.Values {
	values := url.Values{}

	limit := q.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	values.Set("limit", itoa(limit))

	// A cursor already encodes the filters, the field selection and the
	// ordering. Sending them alongside is rejected by some catalog versions,
	// so a cursored query carries nothing else.
	if q.Cursor != "" {
		values.Set("cursor", q.Cursor)
		return values
	}

	for _, filter := range q.Filters {
		if encoded := filter.encode(); encoded != "" {
			values.Add("filter", encoded)
		}
	}
	for _, field := range q.Fields {
		values.Add("fields", field)
	}
	for _, order := range q.OrderBy {
		direction := "asc"
		if order.Descending {
			direction = "desc"
		}
		values.Add("orderField", order.Field+","+direction)
	}
	if q.FullTextTerm != "" {
		values.Set("fullTextFilterTerm", q.FullTextTerm)
		for _, field := range q.FullTextFields {
			values.Add("fullTextFilterFields", field)
		}
	}

	return values
}
