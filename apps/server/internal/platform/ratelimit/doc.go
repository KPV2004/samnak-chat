// Package ratelimit limits how often a key may do something: failed logins per
// email or per IP (AUTH-06), and API calls per user or per client IP (NFR-07).
// See docs/architecture.md section 3.2.
package ratelimit
