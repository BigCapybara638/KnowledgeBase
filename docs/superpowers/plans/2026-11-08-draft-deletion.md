# Draft deletion

**Goal:** Let an author delete a private draft from the drafts list.

**Architecture:** Keep the existing Go service, SQLite transaction, native POST forms, and CSRF middleware. No schema changes or dependencies.

## Requirements

- Only the owner can delete a draft, with no administrator bypass.
- Require the displayed draft version; reject concurrent changes with 409.
- Recheck actor permissions within the write transaction.
- Remove the draft and its media references. Existing media GC remains responsible for unused uploads.
- Remove an unpublished placeholder only when no drafts still reference it.
- Preserve published/archived articles, sibling drafts, and shared media.
- Show a separate delete button beside each draft's edit link, with confirmation.
- POST `/drafts/{id}/delete`, with `version` and CSRF token; redirect to `/drafts` on success.
- Commit and push the completed work to `main`, preserving remote history and excluding local environment files.

## Implementation and verification

1. Add real SQLite service tests for owner deletion, permissions, versions, publications, siblings, and media. Run them failing, then implement `DeleteDraft`.
2. Add browser tests for cancel/delete, published revision removal, foreign-owner requests, stale versions, and CSRF. Run against the previous version to reproduce the missing feature.
3. Add the HTTP handler/route and draft-card form, preserving existing spacing and CSP.
4. Run Go tests, build the application, run the complete browser suite, and review the diff independently.
5. Commit the feature, verify remote `main` ancestry, then push without forcing.
