---
layout: home
title: Samnak Chat docs
hero:
  name: Samnak Chat
  text: Self-hosted team chat with work built in
  tagline: Tasks, approval requests and helpdesk tickets live inside the conversation they come from. You run it on your own server.
  actions:
    - theme: brand
      text: Install on your server
      link: /install
    - theme: alt
      text: User guide
      link: /guide/messenger
features:
  - title: Work in the conversation
    details: Make a task from a message, send an approval request, or open a helpdesk ticket. Each one stays as a card where it was posted.
  - title: Your server, your data
    details: Samnak Chat runs with Docker Compose on one machine. PostgreSQL and file storage are in your own stack.
  - title: Only the modules you need
    details: Tasks, approvals and the helpdesk are optional. Switch each one on with MODULES_ENABLED.
  - title: Thai and English
    details: The web app is in Thai and English, with a light and a dark theme. It works on phones from 360 px wide.
---

## What it is

Samnak Chat is chat for an organization. People talk in channels and direct messages, as in other chat apps. Three kinds of work also live there:

- **Tasks** with an assignee, a due date and a status. Reminders go out when a task is due within 24 hours and when it is overdue.
- **Approval requests**, such as leave or a general request, that the approver accepts or rejects on the card.
- **Helpdesk tickets**, sorted into queues and categories, with an agent who claims each one.

![Messenger in light mode](/images/web/messenger-light.png)

<table>
  <tr>
    <td width="70%"><img src="/images/web/messenger-dark.png" alt="Messenger in dark mode"></td>
    <td width="30%"><img src="/images/web/messenger-mobile.png" alt="Messenger on a phone"></td>
  </tr>
</table>

## Modules

The server is one program made of modules. Four are core and always on. Three are optional, and you choose them in `MODULES_ENABLED`.

| Module | Kind | What it gives you |
| --- | --- | --- |
| `auth` | core | Sign-up, sign-in, email verification, password reset, and the list of signed-in devices |
| `workspace` | core | Workspaces, members and roles, invites, groups, and the audit log |
| `chat` | core | Channels, direct messages, threads, reactions, file attachments, slash commands and realtime delivery |
| `notify` | optional, on by default | The notification bell, per-type switches, muted channels and do-not-disturb hours |
| `task` | optional | Tasks, reminders, and task cards in chat |
| `approval` | optional | Approval requests with approve, reject and cancel, and request cards in chat |
| `helpdesk` | optional | Queues, categories, tickets, claiming and assignment, comments, and ticket cards in chat |

A disabled module is hidden in the web app and its routes return 404. Its data stays in the database, so turning the module back on brings it back. See [Configuration](/configuration#modules).

## What is not in v0.1.0

These are planned. They are not in the release:

- Push notifications on phones and in the browser
- Email from the notification module (mail is sent only for sign-up, verification and password reset)
- A desktop app
- A mobile app
- Helpdesk SLA checks and automatic closing of tickets

The [architecture document](https://github.com/KPV2004/samnak-chat/blob/main/docs/architecture.md) marks everything that is planned.

## Where to go next

- **Running it:** [Install on your server](/install), then [Configuration](/configuration) and [Operate](/operate).
- **Using it:** the [user guide](/guide/messenger) covers each part of the app.
- **Changing it:** the [developer section](/developers/setup) covers setup, architecture and the API.
- **What changed:** the [changelog](/changelog).

Samnak Chat is open source under the [AGPL-3.0](https://github.com/KPV2004/samnak-chat/blob/main/LICENSE) license. The source is on [GitHub](https://github.com/KPV2004/samnak-chat).
