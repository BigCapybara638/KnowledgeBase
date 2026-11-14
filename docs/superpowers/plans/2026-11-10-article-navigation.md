# Article navigation implementation plan

**Goal:** Highlight the current article, add previous/next links below it, return to home from articles, and remove category listing screens. Each change gets its own commit.

**Architecture:** Keep Go templates, HTMX category menus and existing CSS tokens. Category groups and admin category management remain. Previous/next follow the sidebar's `title COLLATE NOCASE, id` order within the current category, excluding unpublished and archived articles.

**Tech stack:** Go, SQLite, HTML templates, HTMX, Vite, Playwright.

**Spec:** User request in this session; these are small changes to the existing article navigation.

## Constraints and review focus

- Preserve menu pagination and active state after loading more articles; show the current article even beyond the initial 50 entries.
- Neighbors must remain deterministic for duplicate titles and absent at category boundaries; a single article has no neighbors.
- Keep authorization and publication visibility rules.
- Old category listing URLs should redirect home; home pagination must not retain category filters.
- Keep navigation usable at 390px and in light/dark themes.

## Tasks

1. Highlight current article: pass its ID through initial and paginated HTMX requests, render `aria-current="page"`, style it with theme tokens, and include the current menu entry on initial load. Verify rendered menu and browser behavior; commit `feat: highlight current article in sidebar`.
2. Add `Service.Neighbors(ctx, actor, id)` and bottom navigation. Test category boundaries, equal/case-insensitive titles, hidden articles, >50 entries and access denial; verify browser links and narrow layouts; commit `feat: add previous and next article navigation`.
3. Change the article's back link to `/` with the label `Все статьи`. Verify return destination; commit `fix: return from articles to home`.
4. Remove category filter links, home filter handling and category-specific pagination; redirect old filter URLs to `/`. Keep admin category CRUD and grouped article menu. Verify home pagination and redirects; commit `refactor: remove category listing screens`.

For each task, run focused regression checks before committing. Run the complete Go suite, vet, UI build and browser suite before completion, then review the complete diff.

## Verification results

- Tasks 1–4 implemented; regression checks observed failing before implementation and passing afterwards for active menus, neighbor selection/rendering, and removed category listings.
- Complete `go test -trimpath ./...` and `go vet ./...` passed in the Go 1.27.3 Docker environment.
- Production Vite and Go builds passed. Existing HTMX eval build warning remains.
- Complete Playwright suite passed: 21/21 in Edge, using an isolated Docker database. Navigation also checked at 1440px/390px in light/dark themes; screenshots are under `.validation/navigation-*.png`.
- Independent review found no Critical/Important issues. Optional follow-ups: auto-scroll the menu to its active item; browser coverage of actual load-more with 100+ entries (current HTTP coverage checks pagination and active state beyond 50).
