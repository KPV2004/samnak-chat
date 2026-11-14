# 0007: Per-channel `seq` and `client_msg_id`

## Status

Accepted, 2026-11-14

## Context

Chat has three message problems ([architecture.md](../architecture.md) section 9.1):

- A network drop can cause a client to resend, which must not create a duplicate message.
- Devices must show messages in the same order.
- A client that was offline must catch up on what it missed.

The SRS requires that messages are neither lost nor duplicated when the network drops or a send is retried ([NFR-09](../srs.md)). The data model enforces uniqueness on `(channel_id, seq)` and on `(sender_id, client_msg_id)` ([srs.md](../srs.md) section 13). The WebSocket does not replay history, so catching up goes through REST `messages?after_seq=` ([srs.md](../srs.md) section 14.3).

## Decision

- The client generates a `client_msg_id` for each message and reuses it on every retry. The database rejects a second message with the same sender and `client_msg_id`.
- The server assigns `seq` per channel, increasing by one for each new message. The database rejects a duplicate `(channel_id, seq)`.
- After reconnecting, a client calls `messages?after_seq=` for each channel to fill the gap.

## Consequences

- A retry with the same `client_msg_id` must return the message that already exists, not insert a new one. The use case has to handle the uniqueness error this way.
- Every device sees the same order, because order comes from `seq`, not from client clocks.
- Writes to one channel are serialized by its sequence counter. Different channels do not contend with each other.
- Clients must store the last `seq` they have seen for each channel to catch up correctly.

## Alternatives considered

- **One global sequence across all channels.** Simpler to allocate, but all writes contend on one counter and per-channel order is no longer contiguous. Rejected.
- **Order by timestamp.** Clocks on different devices and servers disagree, and two messages can share a timestamp. Rejected.
- **Deduplicate by server-generated ID only.** The client cannot tell whether a lost response meant the message was stored, so a resend creates a duplicate or the client must guess. Rejected because it does not meet [NFR-09](../srs.md).
- **Order by time-sortable IDs such as UUIDv7 alone.** IDs are time-ordered ([architecture.md](../architecture.md) section 8.2), but they are not contiguous per channel, so gaps cannot be detected for catch-up. Rejected as the ordering key; IDs stay as identifiers.
