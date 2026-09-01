package backstage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const scaffolderPath = "/api/scaffolder/v2"

// Task is one run of a scaffolder template.
type Task struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	TemplateRef string `json:"templateRef,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	CreatedBy   string `json:"createdBy,omitempty"`
	// URL is where a human can watch the run in the portal.
	URL string `json:"url,omitempty"`
}

// Done reports whether the task has reached a state it will not leave.
func (t Task) Done() bool {
	switch strings.ToLower(t.Status) {
	case "completed", "failed", "cancelled", "skipped":
		return true
	}
	return false
}

// Failed reports whether the task finished badly. A cancelled task counts:
// nothing the caller asked for happened.
func (t Task) Failed() bool {
	switch strings.ToLower(t.Status) {
	case "failed", "cancelled":
		return true
	}
	return false
}

// TaskEvent is one entry in a task's log. The scaffolder emits a "log" event
// per step transition and a final "completion" event carrying the outcome.
type TaskEvent struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	// Error is set on a completion event for a task that failed.
	Error string `json:"error,omitempty"`
	// Output is the template's rendered output, present on a completion
	// event for a task that succeeded. It is the only place it appears: the
	// task object carries the template's *unrendered* output, with the
	// ${{ }} placeholders still in it.
	Output    map[string]interface{} `json:"output,omitempty"`
	CreatedAt string                 `json:"createdAt,omitempty"`
}

// TaskURL is where the portal shows a task.
func (c *client) TaskURL(id string) string {
	return c.baseURL + "/create/tasks/" + id
}

// Scaffold submits a template for execution and returns the task it created.
//
// This is the one call in this package that changes anything. It is
// asynchronous: the task is queued and the returned Task is almost always
// still open. Follow it with Task and TaskEvents.
func (c *client) Scaffold(ctx context.Context, ref EntityRef, values map[string]interface{}) (Task, error) {
	if err := ref.validate(); err != nil {
		return Task{}, err
	}
	if values == nil {
		values = map[string]interface{}{}
	}

	body := map[string]interface{}{
		"templateRef": ref.String(),
		"values":      values,
		// Sent explicitly: the scaffolder distinguishes an absent secrets
		// object from an empty one on some versions.
		"secrets": map[string]interface{}{},
	}

	var response struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, scaffolderPath+"/tasks", body, &response); err != nil {
		return Task{}, err
	}
	if response.ID == "" {
		return Task{}, fmt.Errorf("running %s: the scaffolder returned no task id", ref)
	}

	return Task{
		ID:          response.ID,
		Status:      "open",
		TemplateRef: ref.String(),
		URL:         c.TaskURL(response.ID),
	}, nil
}

// Task reports the current state of a run.
func (c *client) Task(ctx context.Context, id string) (Task, error) {
	var body struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt string `json:"createdAt"`
		CreatedBy string `json:"createdBy"`
		Spec      struct {
			TemplateInfo struct {
				EntityRef string `json:"entityRef"`
			} `json:"templateInfo"`
		} `json:"spec"`
	}
	if err := c.get(ctx, scaffolderPath+"/tasks/"+id, nil, &body); err != nil {
		return Task{}, err
	}

	return Task{
		ID:          body.ID,
		Status:      body.Status,
		TemplateRef: body.Spec.TemplateInfo.EntityRef,
		CreatedAt:   body.CreatedAt,
		CreatedBy:   body.CreatedBy,
		URL:         c.TaskURL(id),
	}, nil
}

// TaskEvents returns the events with an id greater than after, so a caller
// following a run passes back the last id it saw and never re-reads a line.
// Pass 0 for the whole log.
func (c *client) TaskEvents(ctx context.Context, id string, after int) ([]TaskEvent, error) {
	query := url.Values{}
	if after > 0 {
		query.Set("after", itoa(after))
	}

	// The catalog returns {"events": [...]}; some versions return the array
	// directly, so the body is decoded loosely and both shapes accepted.
	var raw json.RawMessage
	if err := c.get(ctx, scaffolderPath+"/tasks/"+id+"/events", query, &raw); err != nil {
		return nil, err
	}

	var wrapper struct {
		Events []rawTaskEvent `json:"events"`
	}
	events := wrapper.Events
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Events != nil {
		events = wrapper.Events
	} else {
		var bare []rawTaskEvent
		if err := json.Unmarshal(raw, &bare); err != nil {
			return nil, fmt.Errorf("decoding the events of task %s: %w", id, err)
		}
		events = bare
	}

	out := make([]TaskEvent, 0, len(events))
	for _, event := range events {
		out = append(out, event.decode())
	}
	return out, nil
}

// rawTaskEvent mirrors the wire shape, whose body is a free-form object.
type rawTaskEvent struct {
	ID        int             `json:"id"`
	Type      string          `json:"type"`
	CreatedAt string          `json:"createdAt"`
	Body      json.RawMessage `json:"body"`
}

func (r rawTaskEvent) decode() TaskEvent {
	event := TaskEvent{ID: r.ID, Type: r.Type, CreatedAt: r.CreatedAt}

	var body struct {
		Message string                 `json:"message"`
		Output  map[string]interface{} `json:"output"`
		Error   struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(r.Body, &body) != nil {
		return event
	}

	event.Message = strings.TrimRight(body.Message, "\n")
	event.Output = body.Output
	if body.Error.Message != "" {
		event.Error = body.Error.Message
		if body.Error.Name != "" {
			event.Error = body.Error.Name + ": " + body.Error.Message
		}
	}
	return event
}
