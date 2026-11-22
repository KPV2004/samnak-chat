# Helpdesk

The helpdesk takes requests from people in the organization and routes each one to the team that can solve it. A person writes a ticket. A queue holds it. An agent of that queue claims it, works on it, and resolves it. This page needs the `helpdesk` module, which is off unless `MODULES_ENABLED` lists it. See [Configuration](/configuration#modules).

## Words used here

- **Queue**: a team's inbox, such as IT, HR or Facilities. Tickets go to one queue.
- **Ticket prefix**: two to six letters that start every ticket number in the queue, such as `IT` for `IT-0142`.
- **Category**: a sub-topic inside a queue. One category of a queue can be its default.
- **Agent**: a member who handles one queue's tickets. A person can be an agent of several queues.
- **Requester**: the person who opened the ticket.

## Open a ticket

Choose **New ticket** in the helpdesk. Pick the queue and the category, then enter:

- a **title** of 1 to 200 characters
- **details**, up to 8,000 characters

You can attach files as well: up to 10 files, each up to 50 MB.

Two other ways open a ticket from chat, when the module is on:

- `/ticket <title>` in a channel. It uses the default category of a queue, so an admin must set one in the helpdesk settings first.
- **Create ticket** in a message's actions. The message becomes the ticket's details.

A new ticket appears on the queue's agents' channel when the queue has one.

## Status

| Status | Meaning |
| --- | --- |
| **New** | Sent, and nobody has picked it up yet |
| **Open** | An agent has claimed it or been assigned to it |
| **In progress** | The assignee is working on it |
| **Waiting** | The assignee is waiting for something, such as an answer from the requester |
| **Resolved** | The assignee has finished the work |
| **Closed** | The ticket is finished. It cannot be reopened or assigned. |

A ticket becomes **Open** when it is claimed or assigned. Only the moves below can change a ticket's status:

| From | To | Who can move it |
| --- | --- | --- |
| Open | In progress | The assignee, or an admin |
| In progress | Waiting | The assignee, or an admin |
| Waiting | In progress | The assignee, or an admin |
| In progress | Resolved | The assignee, or an admin |
| Resolved | Closed | The requester, or an admin |
| Resolved | Open (reopen) | The requester only, within seven days of the ticket being resolved |

Anything else is refused. A closed ticket cannot be reopened.

## Claim and assign

- An **agent** of the queue can **claim** a ticket, but only while it is unassigned or already theirs. An agent cannot assign a ticket to someone else.
- An **admin** can assign any ticket in any queue to an agent of that queue.
- A closed ticket cannot be assigned or claimed.

Claiming a new ticket also makes it Open.

## Priority and category

Priority is **Low**, **Normal**, **High** or **Urgent**. Agents and admins set it, and they also change a ticket's category. A requester cannot.

## Comments

Each ticket has a thread of comments:

- A **comment** is visible to the requester and to the agents.
- An **internal note** is visible only to agents and admins. Tick **Internal note, for agents only** before posting it. Requesters never see it.

Comments can be up to 8,000 characters.

## Who sees a ticket

The requester, the agents of its queue, and admins can see a ticket. Anyone else gets a "not found" answer.

## History

Each ticket has a history of what happened: who opened it, who assigned it, every status, priority and category change, and each comment. It shows who did each change and when.

## Helpdesk settings

Owners and admins open **Helpdesk settings** to set up the helpdesk:

- **Queues.** Create a queue with a name and a ticket prefix. Optionally pick an agents' channel, where every new ticket of the queue gets a card. Archive a queue when it is no longer used. Its tickets stay, but it takes no new ones.
- **Agents.** Choose the members who handle each queue.
- **Categories.** Add categories to a queue, and make one the default. Chat-made tickets use the default.

## Notifications

Notices cover new tickets, tickets assigned to you, status changes and comments on tickets. Each type has its own switch in [notification settings](/guide/notifications).

## Not in v0.1.0

Service-level agreement (SLA) checks, and automatic closing of tickets that have been waiting too long, are planned. They are not built.
