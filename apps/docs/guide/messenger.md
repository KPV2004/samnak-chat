# Messenger

Messenger is the chat part of Samnak Chat: channels, direct messages, threads and reactions. It is the home page of the web app. Tasks, approvals and tickets also appear here as cards.

![Messenger in light mode](/images/web/messenger-light.png)

On a phone the conversation list and the chat are separate screens, and the menu opens from the top bar.

## The conversation list

The left column lists every channel you belong to and every direct message, newest activity first.

- **Pin to top.** Pin a chat to keep it at the top of the list. You can pin up to 30 chats. Pins are yours only.
- **Filters.** Show all conversations, only unread ones, only **Groups** (channels), or only **Direct** messages.
- **Search.** Type in the search box to find a conversation by name.
- **Unread counts.** Each conversation shows how many messages you have not read. Mentions of you are counted separately.
- **Muted.** A muted channel is marked **Muted**. Its messages do not send you notifications. See [Notifications](/guide/notifications).

Messenger finds conversations by name. It does not search message text.

## Channels

A channel is a room for a topic, such as a team or a project.

- **Public** channels are visible to everyone in the workspace. Open **Browse channels** and press **Join**.
- **Private** channels are visible only to the people in them. A member of the channel adds you.

To create a channel, choose **New channel**. Give it a name of 1 to 80 characters, and optionally a topic, a visibility and the people to add. Owners, admins, managers and members can create channels. Guests cannot.

Guests see only the channels they were added to.

A channel's creator, an owner or an admin can change its name and topic, or **archive** it. An archived channel stays readable, but no new messages can be sent to it.

## Direct messages

Choose **New message**, then pick 1 to 8 people. A direct message with one person is a one-to-one conversation. With several people it works as a group chat. Every direct message, one-to-one or with several people, is listed under **Direct**.

If a direct message with the same people already exists, the app opens it instead of creating a second one.

## Writing messages

- Press **Enter** to send, and **Shift+Enter** for a new line.
- A message can have up to 4,000 characters.
- The toolbar adds bold, italic, code and links, and an emoji picker.
- Type `@` and a name to **mention** someone. Mentioned people in the channel are notified. Type `@channel` to notify everyone in the channel.
- Type `/` to see the slash commands. A command appears only when its module is on:
  - `/task <title> [@assignee]` creates a task in this channel. See [Tasks](/guide/tasks).
  - `/ticket <title>` opens a helpdesk ticket in this channel. It uses the default category of a queue, so set one in the helpdesk settings first. See [Helpdesk](/guide/helpdesk).
- **Attach files** with the attach button in the composer. A message can have up to 10 files, and each file can be up to 50 MB. A download link works for 15 minutes; if it expires, open the file again.

Messenger shows who is typing in the channel you have open.

## Your messages

Hover over a message, or focus it with the keyboard, to see its actions.

- **Edit.** You can edit your own text messages. The message then shows **(edited)**.
- **Delete.** You can delete your own messages. Owners and admins can delete anyone's message. A deleted message shows **This message was deleted.** Deleting cannot be undone.
- **React.** Pick an emoji. To take your reaction back, press the same emoji again.
- **Reply in thread.** Opens the thread in a side pane. Replies to a message are counted under it.
- **Create task** and **Create ticket** appear on a message when the task or helpdesk module is on. They turn the message into a card.

Messages cannot be pinned in v0.1.0. Pinning applies to chats in the list, as described above.

## Cards

Tasks, approval requests and helpdesk tickets show as cards in the conversation. You can act on them where they were posted: change a task's status, approve a request, or claim a ticket.

If the module that made a card is turned off, the card shows its plain text instead.

## When the connection drops

Messages arrive live over a WebSocket connection. If the connection drops, the app shows **Connection lost. Reconnecting…** and reconnects by itself. Messages sent while you were away are loaded when you reconnect. A message sent twice after a network drop is still shown once.
