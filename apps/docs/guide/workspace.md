# Workspace

A workspace is one organization's space in Samnak Chat. It has its own channels, members, invites and groups. You can belong to several workspaces and switch between them with the tiles on the left of the app.

## Create a workspace

Choose the dashed tile in the workspace rail, then enter:

- a **workspace name**
- a **short name** (slug) of 3 to 40 characters, using lowercase letters, digits and hyphens. It must be unique.

You become the workspace's **owner**.

## Roles

| Role | What it can do |
| --- | --- |
| **Owner** | Everything an admin can do. Also grants the owner role and changes owners. The last active owner cannot be demoted or deactivated. |
| **Admin** | Manages members, invites and groups, and reads the audit log |
| **Manager** | The same permissions as a member in v0.1.0. Being a manager of a group is a separate setting, and it is what makes a person a default approver. |
| **Member** | Uses channels, creates channels and sends requests |
| **Guest** | Sees and uses only the channels they were added to. Cannot create channels. |

Helpdesk agents are not a workspace role. An agent is a member who is set up as an agent of one or more helpdesk queues. See [Helpdesk](/guide/helpdesk).

Owners and admins manage members, invites and groups. Only an owner can grant or change the owner role.

## Members

Open **Members** to see everyone in the workspace, with their role, their status and when they joined. Owners and admins can:

- **change a role**
- **deactivate** a member, which stops their access without removing them, and **reactivate** them later
- **remove** a member. They lose access to the workspace's channels. You can invite them again later.

Each change is recorded in the audit log.

## Invites

Open **Invites**. Only owners and admins can create and manage invites.

1. Choose the role the person will have: admin, member or guest. An invite cannot give the owner role. A manager is set after the person joins.
2. Optionally enter an email address.
   - **With an email**, only that address can accept the invite.
   - **Without an email**, anyone with the link can accept it.
3. Create the invite and copy the link. The link is shown **once**, so copy it before you leave the page.

An invite is valid for 7 days. Revoke a pending invite from the same page. The invite list never shows the link again.

To accept an invite, open the link and sign in, or sign up first. The page then shows the workspace you are joining. An invite fails when it was sent to a different email address, when it has expired or been revoked, or when you are already a member.

## Groups

**Groups** appears when the approval module is on. A group is a team with managers, whose managers approve its members' requests by default. Owners and admins create groups, name them, set their members and pick their managers. Deleting a group keeps its members in the workspace. See [Approvals](/guide/approvals#groups).

## Audit log

Owners and admins can open **Audit log**. It lists the changes made in the workspace, newest first, with who made each change, when, and what changed:

- invites created and revoked
- members' roles and status changed, and members removed
- groups created, edited and deleted, and their members changed

Members who have left still appear in the log, shown as a former member.
