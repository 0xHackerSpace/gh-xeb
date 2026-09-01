// Package backstage wraps the Backstage Software Catalog API behind a narrow
// interface, so commands depend on this package's types rather than on HTTP
// details and can be tested without a Backstage instance.
//
// Backstage has no official Go SDK -- the catalog is a plain JSON API over
// HTTP -- so this package owns the transport as well as the model. It covers
// the read side of the catalog only: listing entities, fetching one by ref,
// and the kind/type facets. Writing to the catalog happens through git, not
// through this API.
package backstage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds a single catalog request.
	DefaultTimeout = 30 * time.Second

	// DefaultPageSize is what Entities asks for when a query sets no limit.
	// Backstage caps the page size server-side anyway; asking for a sane
	// number keeps a mistyped filter from pulling a whole catalog.
	DefaultPageSize = 100

	// maxPages bounds AllEntities. A server that keeps returning a cursor --
	// through a bug, or a catalog larger than anything a CLI should page
	// through -- stops here instead of looping until the context dies.
	maxPages = 200

	catalogPath = "/api/catalog"
)

// ErrNoBaseURL is returned when no Backstage address is configured.
var ErrNoBaseURL = errors.New("no Backstage base URL")

// ErrNotFound is what a 404 from the catalog unwraps to, so callers can tell
// "this entity does not exist" from "the catalog is unreachable" without
// matching on message text.
var ErrNotFound = errors.New("not found")

// APIError is a non-2xx response from Backstage. The catalog returns a JSON
// body of the shape {"error":{"name","message"},"response":{"statusCode"}};
// when it does not -- a proxy or a load balancer answering instead -- the
// status line is used and a short excerpt of the body is kept.
type APIError struct {
	StatusCode int
	Name       string
	Message    string
	Method     string
	Path       string
}

func (e *APIError) Error() string {
	detail := e.Message
	if detail == "" {
		detail = http.StatusText(e.StatusCode)
	}
	if e.Name != "" && !strings.Contains(detail, e.Name) {
		detail = e.Name + ": " + detail
	}
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.StatusCode, detail)
}

// Unwrap maps the statuses a caller is likely to branch on onto sentinels.
func (e *APIError) Unwrap() error {
	if e.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return nil
}

// Client is the slice of Backstage this extension uses.
type Client interface {
	// BaseURL is the configured address, without a trailing slash.
	BaseURL() string
	// Ping confirms the catalog is reachable and the credentials work.
	Ping(ctx context.Context) error
	// Entities returns one page of entities.
	Entities(ctx context.Context, query EntityQuery) (EntityPage, error)
	// AllEntities follows the cursor until the catalog runs out.
	AllEntities(ctx context.Context, query EntityQuery) ([]Entity, error)
	// Entity fetches a single entity by reference.
	Entity(ctx context.Context, ref EntityRef) (Entity, error)
	// Facets counts entities grouped by one or more fields.
	Facets(ctx context.Context, fields ...string) (map[string][]Facet, error)

	// Scaffold runs a template. It is the only call here that changes
	// anything; see scaffolder.go.
	Scaffold(ctx context.Context, ref EntityRef, values map[string]interface{}) (Task, error)
	// Task reports the current state of a run.
	Task(ctx context.Context, id string) (Task, error)
	// TaskEvents returns a run's log entries with an id greater than after.
	TaskEvents(ctx context.Context, id string, after int) ([]TaskEvent, error)
	// TaskURL is where the portal shows a task.
	TaskURL(id string) string
}

// Config configures a client. Every field is optional: what is left empty is
// taken from the environment, so callers can pass flags through without
// having to decide whether a flag was set.
type Config struct {
	// BaseURL is the Backstage app address, e.g. https://backstage.example.com.
	// The /api/catalog suffix is added per request and must not be included.
	// Falls back to BACKSTAGE_BASE_URL, then BACKSTAGE_URL.
	BaseURL string

	// Token authenticates the request as a bearer token. Falls back to
	// BACKSTAGE_TOKEN. An empty token is allowed -- catalogs that permit
	// unauthenticated reads exist -- and the header is then omitted.
	//
	// This value is a credential: it is never logged, never included in an
	// error, and never rendered by this package.
	Token string

	// HTTPClient overrides the transport, which is how tests reach an
	// httptest server and how a caller injects a proxy or custom TLS.
	HTTPClient *http.Client

	// UserAgent identifies this extension in the catalog's access logs.
	UserAgent string
}

type client struct {
	baseURL   string
	token     string
	http      *http.Client
	userAgent string
}

// New builds a client. It performs no I/O: a wrong address or a dead server
// surfaces on the first call, not here, so that constructing a client in a
// command's setup path cannot block.
func New(cfg Config) (Client, error) {
	raw := firstNonEmpty(cfg.BaseURL, os.Getenv("BACKSTAGE_BASE_URL"), os.Getenv("BACKSTAGE_URL"))
	if raw == "" {
		return nil, ErrNoBaseURL
	}

	base, err := normalizeBaseURL(raw)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	return &client{
		baseURL:   base,
		token:     firstNonEmpty(cfg.Token, os.Getenv("BACKSTAGE_TOKEN")),
		http:      httpClient,
		userAgent: firstNonEmpty(cfg.UserAgent, "gh-cli-extension"),
	}, nil
}

// normalizeBaseURL accepts what a user is likely to paste -- with or without a
// scheme, with or without a trailing slash -- and rejects what cannot work.
func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		// A bare host is unambiguous enough to fix rather than reject, but
		// assume TLS: defaulting to plaintext would silently send a bearer
		// token in the clear.
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parsing Backstage URL %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("Backstage URL %q must be http or https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("Backstage URL %q has no host", raw)
	}

	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""

	// A pasted address often already points at the catalog API. Keeping the
	// suffix would produce /api/catalog/api/catalog/entities.
	u.Path = strings.TrimSuffix(u.Path, catalogPath)
	u.Path = strings.TrimSuffix(u.Path, "/api")

	return u.String(), nil
}

func (c *client) BaseURL() string { return c.baseURL }

func (c *client) Ping(ctx context.Context) error {
	// entity-facets is the cheapest authenticated endpoint the catalog plugin
	// exposes: it answers from the index without returning entity bodies, and
	// it fails the same way a real query would if the token is wrong. There
	// is no catalog health endpoint to use instead.
	_, err := c.Facets(ctx, "kind")
	return err
}

func (c *client) Entities(ctx context.Context, query EntityQuery) (EntityPage, error) {
	var body struct {
		Items      []Entity `json:"items"`
		TotalItems int      `json:"totalItems"`
		PageInfo   struct {
			NextCursor string `json:"nextCursor"`
			PrevCursor string `json:"prevCursor"`
		} `json:"pageInfo"`
	}

	if err := c.get(ctx, catalogPath+"/entities/by-query", query.values(), &body); err != nil {
		return EntityPage{}, err
	}

	return EntityPage{
		Items:      body.Items,
		TotalItems: body.TotalItems,
		NextCursor: body.PageInfo.NextCursor,
		PrevCursor: body.PageInfo.PrevCursor,
	}, nil
}

func (c *client) AllEntities(ctx context.Context, query EntityQuery) ([]Entity, error) {
	var all []Entity

	for page := 0; page < maxPages; page++ {
		got, err := c.Entities(ctx, query)
		if err != nil {
			return nil, err
		}
		all = append(all, got.Items...)

		if got.NextCursor == "" {
			return all, nil
		}
		// Past the first request the cursor carries the filters, and sending
		// them again is an error on some versions. Only the cursor travels.
		query = EntityQuery{Cursor: got.NextCursor, Limit: query.Limit}
	}

	return all, fmt.Errorf("listing entities: still paginating after %d pages, giving up", maxPages)
}

func (c *client) Entity(ctx context.Context, ref EntityRef) (Entity, error) {
	if err := ref.validate(); err != nil {
		return Entity{}, err
	}

	path := strings.Join([]string{
		catalogPath, "entities", "by-name",
		url.PathEscape(ref.Kind),
		url.PathEscape(ref.Namespace),
		url.PathEscape(ref.Name),
	}, "/")

	var entity Entity
	if err := c.get(ctx, path, nil, &entity); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Entity{}, fmt.Errorf("no entity %s in the catalog: %w", ref, ErrNotFound)
		}
		return Entity{}, err
	}
	return entity, nil
}

// Facet is one value of a grouped field and how many entities carry it.
type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func (c *client) Facets(ctx context.Context, fields ...string) (map[string][]Facet, error) {
	if len(fields) == 0 {
		return nil, errors.New("no facet field requested")
	}

	query := url.Values{}
	for _, field := range fields {
		query.Add("facet", field)
	}

	var body struct {
		Facets map[string][]Facet `json:"facets"`
	}
	if err := c.get(ctx, catalogPath+"/entity-facets", query, &body); err != nil {
		return nil, err
	}
	return body.Facets, nil
}

// get performs a GET and decodes the JSON body into out.
func (c *client) get(ctx context.Context, path string, query url.Values, out interface{}) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

// post performs a POST with a JSON body and decodes the response into out.
func (c *client) post(ctx context.Context, path string, body, out interface{}) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding the request for %s: %w", path, err)
	}
	return c.do(ctx, http.MethodPost, path, nil, encoded, out)
}

// do performs one request and decodes the JSON body into out.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body []byte, out interface{}) error {
	target := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// The URL is safe to show; the Authorization header is not, and is
		// not part of err.
		return fmt.Errorf("requesting %s: %w", path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiError(resp, method, path)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding the response from %s: %w", path, err)
	}
	return nil
}

// apiError turns a failed response into an *APIError, reading the catalog's
// JSON error envelope when there is one.
func apiError(resp *http.Response, method, path string) error {
	err := &APIError{StatusCode: resp.StatusCode, Method: method, Path: path}

	// Bounded: an HTML error page from a proxy can be arbitrarily large, and
	// none of it belongs in a terminal.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))

	var envelope struct {
		Error struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
		err.Name = envelope.Error.Name
		err.Message = envelope.Error.Message
		return err
	}

	if excerpt := excerpt(raw); excerpt != "" {
		err.Message = excerpt
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// The most common cause by far, and invisible from the status alone.
		err.Message = strings.TrimSpace(err.Message + " (check BACKSTAGE_TOKEN)")
	}

	return err
}

// excerpt reduces a response body to a single short line fit for an error.
func excerpt(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	if i := strings.IndexAny(text, "\r\n"); i >= 0 {
		text = text[:i]
	}
	const limit = 160
	if len(text) > limit {
		text = text[:limit] + "..."
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// itoa keeps the query builder in entity.go free of strconv noise.
func itoa(n int) string { return strconv.Itoa(n) }
