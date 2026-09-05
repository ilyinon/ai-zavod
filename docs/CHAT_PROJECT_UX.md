# V1.0.7 - Chats & Projects UX

## Purpose

A conversation is independent from a workspace. A user can start typing after
one click on New Chat, without naming a project or opening settings.

## Behaviour

- The global plus creates a persistent, unnamed chat without a project.
- A project plus creates a separate chat in that project.
- The first user message supplies an editable title.
- General questions and web research work without a project. Research sources
  are persisted with the chat and are available after reopening it.
- File-producing tasks pause before workflow execution until the user chooses a
  workspace. The user can connect an existing project, select a directory using
  the macOS folder picker, or create a directory with a suggested name.
- Attaching the workspace resumes the original request without a second user
  message. No files are generated before workspace selection.
- The composer provides project, group and model selectors. An empty group/model
  choice uses automatic routing/configured defaults; explicit choices belong to
  the chat, not to the entire project.
- Chats can be renamed, pinned, archived, restored and deleted. Search covers
  chat titles and project names. Unsent drafts survive chat switching in-session.
- Deleting a chat removes its database history, not project files. Removing a
  project keeps its chats as unbound conversations and leaves disk files intact.
- A chat with a file workflow cannot currently move between workspaces: its
  stored diffs and rollback data belong to the original directory. Ordinary
  conversations can move between projects or become projectless.

## Data and compatibility

Existing tasks are reused as chats. The migration preserves task and message IDs,
adds nullable project ownership, pinned state, pending workspace requests and
per-chat group/model preferences. Project deletion uses SET NULL for chat
ownership. Migration runs transactionally with a foreign-key integrity check.

The Wails API exposes CreateChat, SelectChat, ListChats, UpdateChat, DeleteChat and
ChooseProjectFolder. SendMessage accepts an explicit taskId. Task context follows
execution independently of the selected UI chat. Review gates use the task's
own spec; unrelated conversation artifacts are excluded from direct-answer context.

## Execution

One request per chat is accepted at a time. Requests from chats sharing a project
are serialized in-process. The UI shows the queued state. Different projects may
run independently. Manual apply, rollback, review and test operations use the same
project queue. Cancellation removes the queued entry.

This release does not add Git worktrees or durable background job scheduling.

## Verification

- Store tests cover migration/reopening, ownership, isolation and deletion.
- App tests cover explicit chat selection, projectless answers, workspace gates
  and project execution queues.
- `scripts/chats-ui-smoke.cjs` covers navigation, background events, drafts,
  workspace selection, rename/archive and desktop/mobile layouts with a mock
  Wails bridge. Run it against `npm run dev --prefix frontend -- --port 5179`
  using a Node environment with Playwright and Chromium installed.
- The native app is built and packaged with `make build`.
