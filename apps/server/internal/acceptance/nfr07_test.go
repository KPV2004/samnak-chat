package acceptance

import (
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// presignLifetime is how long a presigned upload or download URL works (NFR-07).
const presignLifetime = 15 * time.Minute

// TestPresignedURLLifetime is NFR-07: every presigned URL the API issues works
// for 15 minutes. Uploads and downloads of chat, helpdesk and approval each
// get a case. A URL is checked two ways: X-Amz-Expires in the signed URL, and
// expires_at in the reply, which the server stamps between the request and the
// reply.
func TestPresignedURLLifetime(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	in := newInstanceWith(t, pool, rdb, allModulesConfig(testStorage(t)))
	c := client{t: t, url: in.url}

	alice := c.signUp("alice")
	ws := c.createWorkspace(alice)
	channel := c.createChannel(alice, ws, unique("nfr07-room"), nil)

	// check asserts one presigned object: rawURL and expiresAt as the reply has
	// them, for a request made between before and after.
	check := func(t *testing.T, rawURL, expiresAt string, before, after time.Time) {
		t.Helper()
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("presigned URL %q: %v", rawURL, err)
		}
		if got := u.Query().Get("X-Amz-Expires"); got != "900" {
			t.Errorf("X-Amz-Expires = %q, want 900 (15 minutes)", got)
		}
		exp, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			t.Fatalf("expires_at %q: %v", expiresAt, err)
		}
		if exp.Before(before.Add(presignLifetime)) || exp.After(after.Add(presignLifetime)) {
			t.Errorf("expires_at = %s, want 15 minutes after the request, between %s and %s",
				exp.Format(time.RFC3339Nano), before.Add(presignLifetime).Format(time.RFC3339Nano),
				after.Add(presignLifetime).Format(time.RFC3339Nano))
		}
	}

	t.Run("chat upload", func(t *testing.T) {
		sc := client{t: t, url: in.url}
		before := time.Now()
		r := sc.want(sc.do("POST", "/chat/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
			"channel_id": channel, "file_name": "notes.txt", "size": 12, "mime_type": "text/plain",
		}}), 201)
		after := time.Now()
		up := presignedObject(t, r.Body["upload"])
		check(t, up["url"], up["expires_at"], before, after)
	})

	t.Run("chat download", func(t *testing.T) {
		sc := client{t: t, url: in.url}
		id := sc.want(sc.do("POST", "/chat/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
			"channel_id": channel, "file_name": "notes.txt", "size": 12, "mime_type": "text/plain",
		}}), 201).str("attachment_id")
		before := time.Now()
		r := sc.want(sc.do("GET", "/chat/attachments/"+id, call{token: alice.access, workspace: ws}), 200)
		after := time.Now()
		check(t, r.str("url"), r.str("expires_at"), before, after)
	})

	t.Run("helpdesk upload", func(t *testing.T) {
		sc := client{t: t, url: in.url}
		before := time.Now()
		r := sc.want(sc.do("POST", "/helpdesk/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
			"file_name": "log.txt", "size": 10, "mime_type": "text/plain",
		}}), 201)
		after := time.Now()
		up := presignedObject(t, r.Body["upload"])
		check(t, up["url"], up["expires_at"], before, after)
	})

	t.Run("helpdesk download", func(t *testing.T) {
		sc := client{t: t, url: in.url}
		id := sc.want(sc.do("POST", "/helpdesk/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
			"file_name": "log.txt", "size": 10, "mime_type": "text/plain",
		}}), 201).str("attachment_id")
		before := time.Now()
		r := sc.want(sc.do("GET", "/helpdesk/attachments/"+id, call{token: alice.access, workspace: ws}), 200)
		after := time.Now()
		check(t, r.str("url"), r.str("expires_at"), before, after)
	})

	t.Run("approval upload", func(t *testing.T) {
		sc := client{t: t, url: in.url}
		before := time.Now()
		r := sc.want(sc.do("POST", "/approval/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
			"file_name": "quote.pdf", "size": 20, "mime_type": "application/pdf",
		}}), 201)
		after := time.Now()
		up := presignedObject(t, r.Body["upload"])
		check(t, up["url"], up["expires_at"], before, after)
	})

	t.Run("approval download", func(t *testing.T) {
		sc := client{t: t, url: in.url}
		id := sc.want(sc.do("POST", "/approval/uploads", call{token: alice.access, workspace: ws, body: map[string]any{
			"file_name": "quote.pdf", "size": 20, "mime_type": "application/pdf",
		}}), 201).str("attachment_id")
		before := time.Now()
		r := sc.want(sc.do("GET", "/approval/attachments/"+id, call{token: alice.access, workspace: ws}), 200)
		after := time.Now()
		check(t, r.str("url"), r.str("expires_at"), before, after)
	})
}

// presignedObject reads the object an upload reply nests under "upload", and
// fails the test when the URL or its expiry is missing.
func presignedObject(t *testing.T, v any) map[string]string {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("upload reply has no presigned object: %v", v)
	}
	out := map[string]string{}
	for _, key := range []string{"url", "expires_at"} {
		s, _ := m[key].(string)
		if s == "" {
			t.Fatalf("presigned object has no %s: %v", key, m)
		}
		out[key] = s
	}
	return out
}

// TestInputLimits is the NFR-07 input audit over the served spec. Every value a
// client sends should have a limit: a string needs maxLength, enum or format,
// a number needs maximum or enum, an array needs maxItems, and a free-form
// object needs maxProperties. Request bodies and query parameters are audited.
// Path parameters are left out: they are matched against stored IDs.
//
// M6-09 changes no endpoint. The gaps the spec has today are listed in
// knownLimitGaps and reported in the M6-09 report. The test fails for a gap
// that is not listed, and for a listed gap that the spec now limits, so the
// list stays exact.
func TestInputLimits(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	in := newInstanceWith(t, pool, rdb, allModulesConfig(testStorage(t)))

	got := inputLimitGaps(specDocument(t, in.api))
	for _, id := range sortedKeys(got) {
		for _, gap := range got[id] {
			if !slices.Contains(knownLimitGaps[id], gap) {
				t.Errorf("%s: %s has no limit; add maxLength, maximum, maxItems or maxProperties to the schema, or list it in knownLimitGaps", id, gap)
			}
		}
	}
	for _, id := range sortedKeys(knownLimitGaps) {
		for _, gap := range knownLimitGaps[id] {
			if !slices.Contains(got[id], gap) {
				t.Errorf("%s: %s is listed in knownLimitGaps, but the spec now limits it; remove the entry", id, gap)
			}
		}
	}
	total := 0
	for _, gaps := range got {
		total += len(gaps)
	}
	t.Logf("%d operations have %d values without a limit", len(got), total)
}

// knownLimitGaps is the input audit's result at M6-09: each operation with the
// values that have no limit in the served spec. Each entry reads "<place>:
// <problem>", the same text inputLimitGaps reports.
var knownLimitGaps = map[string][]string{
	"approval-create-request": {
		"body.approver_id: string without maxLength",
		"body.attachment_ids[]: string without maxLength",
		"body.payload: object without maxProperties",
	},
	"approval-list-requests": {
		"query.cursor: string without maxLength",
		"query.status: string without maxLength",
	},
	"approval-reject-request": {
		"body.reason: string without maxLength",
	},
	"auth-update-me": {
		"body.timezone: string without maxLength",
	},
	"chat-add-members": {
		"body.user_ids[]: string without maxLength",
	},
	"chat-create-channel": {
		"body.member_ids[]: string without maxLength",
		"body.name: string without maxLength",
		"body.topic: string without maxLength",
	},
	"chat-edit-message": {
		"body.body: string without maxLength",
	},
	"chat-list-messages": {
		"query.after_seq: number without maximum",
		"query.before_seq: number without maximum",
		"query.thread_root_id: string without maxLength",
	},
	"chat-mark-read": {
		"body.seq: number without maximum",
	},
	"chat-open-dm": {
		"body.user_ids[]: string without maxLength",
	},
	"chat-request-upload": {
		"body.channel_id: string without maxLength",
	},
	"chat-run-command": {
		"body.args: string without maxLength",
		"body.name: string without maxLength",
		"body.thread_root_id: string without maxLength",
	},
	"chat-send-message": {
		"body.attachment_ids[]: string without maxLength",
		"body.body: string without maxLength",
		"body.thread_root_id: string without maxLength",
	},
	"chat-update-channel": {
		"body.name: string without maxLength",
		"body.topic: string without maxLength",
	},
	"helpdesk-add-comment": {
		"body.body: string without maxLength",
	},
	"helpdesk-assign-ticket": {
		"body.assignee_id: string without maxLength",
	},
	"helpdesk-create-category": {
		"body.name: string without maxLength",
		"body.queue_id: string without maxLength",
	},
	"helpdesk-create-queue": {
		"body.channel_id: string without maxLength",
		"body.name: string without maxLength",
		"body.prefix: string without maxLength",
	},
	"helpdesk-create-ticket": {
		"body.attachment_ids[]: string without maxLength",
		"body.category_id: string without maxLength",
		"body.description: string without maxLength",
		"body.source_message_id: string without maxLength",
		"body.title: string without maxLength",
	},
	"helpdesk-edit-ticket": {
		"body.category_id: string without maxLength",
	},
	"helpdesk-list-tickets": {
		"query.assignee: string without maxLength",
		"query.cursor: string without maxLength",
		"query.queue_id: string without maxLength",
		"query.status: string without maxLength",
	},
	"helpdesk-set-queue-agents": {
		"body.user_ids: array without maxItems",
		"body.user_ids[]: string without maxLength",
	},
	"helpdesk-update-category": {
		"body.name: string without maxLength",
		"body.queue_id: string without maxLength",
	},
	"helpdesk-update-queue": {
		"body.channel_id: string without maxLength",
		"body.name: string without maxLength",
	},
	"notify-list-notifications": {
		"query.before: string without maxLength",
	},
	"notify-set-preferences": {
		"body.disabled_kinds: array without maxItems",
		"body.disabled_kinds[]: string without maxLength",
		"body.dnd_end: string without maxLength",
		"body.dnd_start: string without maxLength",
		"body.muted_channel_ids: array without maxItems",
		"body.muted_channel_ids[]: string without maxLength",
	},
	"task-create-item": {
		"body.assignee_id: string without maxLength",
		"body.channel_id: string without maxLength",
		"body.source_message_id: string without maxLength",
	},
	"task-list-items": {
		"query.channel_id: string without maxLength",
		"query.cursor: string without maxLength",
		"query.status: string without maxLength",
	},
	"task-update-item": {
		"body.assignee_id: string without maxLength",
	},
	"workspace-create": {
		"body.name: string without maxLength",
		"body.slug: string without maxLength",
	},
	"workspace-create-group": {
		"body.name: string without maxLength",
	},
	"workspace-create-invite": {
		"body.email: string without maxLength",
	},
	"workspace-list-audit": {
		"query.cursor: string without maxLength",
	},
	"workspace-list-members": {
		"query.cursor: string without maxLength",
	},
	"workspace-rename-group": {
		"body.name: string without maxLength",
	},
	"workspace-set-group-members": {
		"body.members[].user_id: string without maxLength",
	},
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// inputLimitGaps returns, per operationId, the places in its request body and
// query parameters that have no limit. Each entry reads "<place>: <problem>".
func inputLimitGaps(doc map[string]any) map[string][]string {
	gaps := map[string][]string{}
	paths, _ := doc["paths"].(map[string]any)
	for _, item := range paths {
		methods, _ := item.(map[string]any)
		for _, raw := range methods {
			op, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id, _ := op["operationId"].(string)
			var out []string
			params, _ := op["parameters"].([]any)
			for _, p := range params {
				param, _ := p.(map[string]any)
				if in, _ := param["in"].(string); in != "query" {
					continue
				}
				name, _ := param["name"].(string)
				schema, _ := param["schema"].(map[string]any)
				limitGaps(doc, "query."+name, schema, &out, 0)
			}
			if rb, ok := op["requestBody"].(map[string]any); ok {
				content, _ := rb["content"].(map[string]any)
				media, _ := content["application/json"].(map[string]any)
				schema, _ := media["schema"].(map[string]any)
				limitGaps(doc, "body", schema, &out, 0)
			}
			if len(out) > 0 {
				sort.Strings(out)
				gaps[id] = out
			}
		}
	}
	return gaps
}

// limitGaps appends the places in schema s, named path, that have no limit.
// depth stops a recursive schema from looping.
func limitGaps(doc map[string]any, path string, s map[string]any, out *[]string, depth int) {
	if s == nil || depth > 16 {
		return
	}
	if ref, ok := s["$ref"].(string); ok {
		limitGaps(doc, path, resolveSchema(doc, ref), out, depth+1)
		return
	}
	switch schemaType(s) {
	case "string":
		if !hasAny(s, "maxLength", "enum", "format") {
			*out = append(*out, path+": string without maxLength")
		}
	case "integer", "number":
		if !hasAny(s, "maximum", "enum") {
			*out = append(*out, path+": number without maximum")
		}
	case "array":
		if !hasAny(s, "maxItems") {
			*out = append(*out, path+": array without maxItems")
		}
		items, _ := s["items"].(map[string]any)
		limitGaps(doc, path+"[]", items, out, depth+1)
	case "object":
		props, _ := s["properties"].(map[string]any)
		names := make([]string, 0, len(props))
		for name := range props {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			prop, _ := props[name].(map[string]any)
			limitGaps(doc, path+"."+name, prop, out, depth+1)
		}
		if ap, ok := s["additionalProperties"]; ok && ap != false && !hasAny(s, "maxProperties") {
			*out = append(*out, path+": object without maxProperties")
		}
	}
}

// schemaType is the schema's type. OpenAPI 3.1 writes a nullable type as a list,
// such as ["string", "null"], so the null is skipped.
func schemaType(s map[string]any) string {
	switch t := s["type"].(type) {
	case string:
		return t
	case []any:
		for _, v := range t {
			if name, _ := v.(string); name != "null" {
				return name
			}
		}
	}
	return ""
}

// hasAny reports whether schema s sets one of the keys.
func hasAny(s map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := s[k]; ok {
			return true
		}
	}
	return false
}

// resolveSchema follows a "#/components/schemas/Name" reference.
func resolveSchema(doc map[string]any, ref string) map[string]any {
	const prefix = "#/components/schemas/"
	name, found := strings.CutPrefix(ref, prefix)
	if !found {
		return nil
	}
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	s, _ := schemas[name].(map[string]any)
	return s
}
