// Package notify stores the notifications other modules' rules produce and
// delivers them to their users (SRS section 8). This file and events.go are
// the only parts other modules may import.
//
// Other modules never call notify. They register a rule in Init through
// app.NotificationRegistry (Deps.Notifications) and publish the event it
// listens for (NOTI-01). With notify disabled nobody reads the rules, so
// those modules work exactly as before (NOTI-05).
package notify
