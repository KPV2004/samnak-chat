// Package jobs implements the background job queue (PLT-09) on River
// (architecture section 10, ADR 0006).
//
// One River client runs per server on the shared pgx pool. It starts after
// every module's Init has registered handlers and schedules, and stops after
// HTTP has drained (NFR-11). Kinds are dynamic strings, so one generic argument
// type carries {kind, json} and the worker dispatches to the handler registered
// for that kind.
//
// A job inserted inside database.TxManager.Do joins that transaction, so it
// exists only if the data it depends on commits. Jobs whose kind has no worker
// on this server are cancelled at start and once per sweep interval.
package jobs
