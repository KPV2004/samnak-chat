# Tasks

Tasks turn a message into work that someone owns. A task has a title, an assignee, an optional due date, a priority and a status. This page needs the `task` module, which is off unless `MODULES_ENABLED` lists it. See [Configuration](/configuration#modules).

## Create a task

There are three ways:

- **From a message.** Open the message's actions and choose **Create task**. The title is taken from the form, and you can pick the assignee, the due date and the priority.
- **With a command.** In any channel, type `/task` and the title. To assign it, type `@` after the title and pick the person from the list, for example `/task Get three quotes for 12 laptops`, then `@Mali`.
- **From a helpdesk ticket.** Open the ticket and choose **Create task**. See [Helpdesk](/guide/helpdesk).

The task appears as a card in the channel where it was made. The card shows the title, the assignee, the priority, the due date and the status.

## Fields

| Field | Rules |
| --- | --- |
| Title | 1 to 200 characters |
| Assignee | Optional. An unassigned task is not in anyone's **My tasks**. |
| Due date | Optional |
| Priority | Low, Normal or High |
| Status | To do, Doing or Done |

## Change a task

Change the status with the select on the task card. Only the task's creator, its assignee, an owner or an admin can edit a task or change its status.

Assigning someone sends them a notification in the **Tasks assigned to you** type. See [Notifications](/guide/notifications).

## My tasks

**My tasks** lists the tasks assigned to you, across every channel. The page has the same filters as the links in its sidebar:

- **My tasks** shows open tasks: To do and Doing. This is the default view.
- **Due today** shows tasks due today.
- **Overdue** shows open tasks whose due date has passed.
- **Done** shows finished tasks.
- **By channel** lists the channels that hold your open tasks, with a count for each.

The toolbar filters by status, by due date (Overdue, Due today, or Due within 7 days) and by channel. Use **Clear filters** to go back to the defaults.

## Reminders

The server checks for due tasks every 5 minutes. For each task that is not done, has an assignee and has a due date, it sends the assignee a notification, once for each due date:

- when the task is due within 24 hours, under **Tasks due soon**
- when the task is overdue, under **Overdue tasks**

You can switch off either type in [notification settings](/guide/notifications). Reminders come through the app's notification bell and in realtime while the app is open. Push and email reminders are planned, not built in v0.1.0.
