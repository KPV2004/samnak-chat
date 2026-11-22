# Approvals

An approval request asks one person to decide on something: a leave day, a purchase, or any other request. The requester fills in a form, the approver approves or rejects it, and both sides see the result on the card in chat. This page needs the `approval` module, which is off unless `MODULES_ENABLED` lists it. See [Configuration](/configuration#modules).

## Send a request

Open **Approvals** and choose **New request**. Pick a type:

**Leave**

| Field | Rules |
| --- | --- |
| Leave type | Annual leave, sick leave, personal leave or other |
| Start date and end date | The end date cannot be before the start date |
| Half day | Full day, morning or afternoon. A half day needs a single day. |
| Reason | Required, up to 1,000 characters |

**General request**

| Field | Rules |
| --- | --- |
| Subject | Required, one line, up to 200 characters |
| Details | Optional, up to 4,000 characters |
| Amount | Optional. A number such as `1200.50`, with at most two decimal places |
| Currency | A three-letter code. It defaults to `THB` when you give an amount. |

Both types accept files. A request can have up to 10 files, and each file can be up to 50 MB.

### Who decides

The request goes to the approver you name, who must be an active member of the workspace other than yourself. If you name nobody, the request goes to your group's manager. The form shows that default as **Automatic: my group's manager**. If you are in no group with a manager, you have to name an approver.

Groups and managers are set up by owners and admins. See [Groups](#groups) below.

## Decide a request

The request appears as a card in a direct message between you and the requester, and in **Waiting for me**. The card has **Approve** and **Reject** buttons, and an **Open request** link.

- **Approve.** The request becomes **Approved**. The requester is notified.
- **Reject.** You must give a reason. The requester sees it, and it stays in the request's history. The request becomes **Rejected**.

A decision is final. Once a request is approved, rejected or cancelled, no one can change its status.

Only the approver can approve or reject, and only while the request is waiting.

## Cancel a request

The requester can **Cancel request** while it is waiting. A cancelled request is final too.

## Lists and status

- **Waiting for me** shows the requests you have to decide. The sidebar shows how many there are.
- **My requests** shows the requests you have sent.

Each request has a status: **Waiting**, **Approved**, **Rejected** or **Cancelled**. Filter the list by status.

Only the requester and the approver can see a request. Anyone else gets a "not found" answer.

## History

Each request has a history: who sent it, who approved, rejected or cancelled it, when, and the reason for a rejection. Attachments can be downloaded from the request page.

## Notifications

- The approver is notified when a request is sent to them, under **Approval requests**.
- The requester is notified when the request is approved, rejected or cancelled, under **Approved requests**, **Rejected requests** or **Cancelled requests**.

You can switch each type off in [notification settings](/guide/notifications).

## Groups

**Groups** is in the Workspace section when the approval module is on. A group is a team whose manager approves its members' requests by default. Owners and admins create groups, rename or delete them, set their members, and pick their managers. A member of a group needs to do nothing: the default approver follows the group.

Deleting a group does not remove its members from the workspace. Only the group goes.
