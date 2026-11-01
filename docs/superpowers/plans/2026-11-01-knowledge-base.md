# Корпоративная база знаний Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать внутреннюю базу знаний с визуальным редактором, личными черновиками и безопасной конкурентной публикацией, работающую в отдельном Compose-проекте в пределах общего бюджета 1 ГБ ОЗУ.

**Architecture:** Один постоянно работающий Go-процесс отдаёт HTML/HTMX, статические ресурсы и защищённые изображения; SQLite находится на локальном томе. Существующий Nginx из другого Compose-проекта направляет запросы через отдельную внешнюю Docker-сеть. Автоматического выключения, активатора и второго Nginx нет.

**Tech Stack:** Go, chi/v5, html/template, github.com/ncruces/go-sqlite3, sqlc, goose, FTS5; HTMX, Tailwind CSS, Tiptap; Docker Compose. Node.js нужен только при сборке.

**Spec:** [2026-11-01-knowledge-base-design.md](../specs/2026-11-01-knowledge-base-design.md). Читать спецификацию и план вместе. Предлагаемые настройки из раздела 3 спецификации не выдавать за уже подтверждённые пользователем требования.

## Global Constraints

- Работать в существующем изолированном checkout; новый worktree не создавать без явного запроса пользователя.
- Go не ниже 1.26; Node.js 24 LTS только при сборке; HTMX 2, Tailwind CSS 4, Tiptap 3 с открытыми расширениями. Зафиксировать patch-версии и lock-файлы.
- Один сервер Linux, один экземпляр приложения, локальная SQLite; не NFS и не общий файл для нескольких контейнеров приложения.
- WAL, `foreign_keys=ON`, `busy_timeout=5000`; одно соединение записи, четыре чтения, ожидание записи до 5 секунд.
- Контейнер: `mem_limit: 768m`, `memswap_limit: 768m`, `GOMEMLIMIT=512MiB`, `GOMAXPROCS=2`; весь добавленный расход до 1 000 000 000 байт. Контейнер 768 МиБ плюс до 180 МиБ обслуживающему слою: 994 050 048 байт, с проверкой на целевой машине.
- Публикация и FTS атомарны; устаревшие версии возвращают `409 Conflict` и не теряют введённый текст.
- Черновики личные; контент доступен только активным зарегистрированным пользователям. Администратор управляет пользователями/категориями и архивированием.
- JSON до 2 МиБ, глубина до 32, до 10 000 узлов; изображения JPEG/PNG/WebP до 10 МиБ и 25 мегапикселей; multipart до 12 МиБ, не более двух параллельных загрузок.
- bcrypt cost 12, пароль от 12 символов и до 72 байт; session-токен от 32 случайных байт, срок 12 часов; после 5 неудач вход блокируется до конца 10-минутного окна, не более двух параллельных проверок пароля.
- Один постоянно работающий `kb serve`, `restart: unless-stopped`; HTTP shutdown 15 секунд, Compose grace 30 секунд. Запуска по запросу нет.
- Отдельная папка `/opt/knowledgebase`, сеть `knowledgebase-ingress`, алиас `knowledgebase-app:8080`; не публиковать 80/443 и не монтировать Docker socket.
- Ежедневная согласованная копия, 14 последних комплектов; orphan-медиа удаляются после 24 часов, с координацией backup/GC.
- Не изменять действующие клиентские Compose-файлы/серверы автоматически: в этом репозитории подготовить точные примеры подключения, а не угадывать чужую конфигурацию.

## Review Focus

Эти пять классов ошибок включены в тесты соответствующих задач:

1. Две вкладки или два автора сохраняют устаревшую правку: конфликт без потери текста, включая повтор запроса — задачи 4 и 9.
2. Прямой ID черновика/изображения, блокировка пользователя и внешняя ссылка возврата: отсутствие обхода прав — задачи 2 и 5.
3. Кавычки/кириллица/таблица и попытка внедрения HTML или URL со скриптом: безопасное отображение и корректный поиск — задачи 4–6.
4. Смена IP после пересоздания контейнера при работающем Nginx: DNS-восстановление без остановки клиентского проекта — задача 8.
5. Копия во время публикации/сборки мусора, ошибка диска и восстановление: согласованные данные и сохранение прежних копий — задача 7.

## Структура файлов и границы

```text
cmd/kb/main.go                       CLI и запуск, без бизнес-логики
internal/config/                    конфигурация и проверка лимитов
internal/domain/                    ID, модели, команды, типизированные ошибки
internal/storage/                   подключения, блокировки, миграции и репозитории
internal/storage/migrations/         встроенные SQL-миграции goose
internal/storage/queries/            исходные запросы sqlc
internal/storage/dbgen/              сгенерированный код sqlc, не редактируется вручную
internal/auth/                      пользователи, пароли, сессии, CSRF, ограничения входа
internal/categories/                категории и их административные операции
internal/articles/                  чтение, черновики, публикация, архив
internal/content/                   проверка JSON и генерация HTML/обычного текста
internal/media/                     файлы, проверка изображений, права и GC
internal/search/                    безопасный FTS-запрос и выдача
internal/backup/                    согласованная копия, ротация, restore
internal/web/                       маршруты, middleware, HTML/JSON-адаптеры
internal/ui/templates/              базовая страница и шаблоны/фрагменты
internal/ui/static/                 собранные ресурсы для go:embed
web/src/                           исходники Tailwind и Tiptap
tests/e2e/                          браузерные проверки Playwright
tests/integration/                  два изолированных Compose-проекта для теста Nginx
tests/load/                         воспроизводимая проверка памяти/конкурентности
deploy/                            Compose, .env.example и фрагменты существующего Nginx
docs/operations.md                  установка, обновление, backup/restore
```

Модуль Go: `github.com/BigCapybara638/KnowledgeBase`. В `domain` используются `type ID string` для UUID и `type Version int64`; `ErrConflict`, `ErrForbidden`, `ErrNotFound`, `ErrBusy` различаются в HTTP. Domain-типы не зависят от HTTP и SQL.

План описывает будущие команды проверки. Ни одна из них не считается уже выполненной. До начала реализации проверить наличие Go/Node.js нужных версий, Docker Compose и средств браузерного тестирования; отсутствующие инструменты установить штатным способом с проверкой подписей/контрольных сумм. В каждом шаге с тестом сначала убедиться в содержательном провале, затем реализовать поведение; инфраструктурную конфигурацию проверять реальным запуском вместо тестов, копирующих её текст. Коммиты в задачах относятся к будущей реализации.

---

### Task 1: Исполняемый сервер и SQLite с миграциями

**Files:** Create `go.mod`, `go.sum`, `.gitignore`, `sqlc.yaml`, `Makefile`, `cmd/kb/main.go`, `internal/config/config.go`, `internal/domain/{types,errors}.go`, `internal/storage/{store,instance_lock}.go`, `internal/storage/migrations/00001_initial.sql`, `internal/storage/queries/health.sql`, `internal/web/server.go`; tests рядом с пакетами.

**Interfaces:** Produces `config.Load() (config.Config, error)`; `storage.Open(ctx context.Context, dataDir string) (*storage.Store, error)`, `(*Store).Close() error`, `(*Store).Ready(ctx context.Context) error`; `storage.AcquireInstanceLock(dataDir string) (io.Closer, error)`; `web.HealthRoutes(store *storage.Store) http.Handler`. `Store` предоставляет отдельные read/write handles, но бизнес-пакеты используют методы репозиториев, а не общий raw SQL.

- [x] **Step 1:** Написать `TestOpenEnforcesPragmasOnEveryConnection`, `TestMigrationsRepeatable`, `TestSecondServeRejected`, `TestReadinessFailsWithClosedDB`, `TestShutdownDrainsRequest`. Проверять FK на соединениях, сохранение строки после reopen и отсутствие готовности при недоступной БД, не только HTTP-статус без обращения к БД.
- [x] **Step 2:** Запустить `go test ./internal/config ./internal/storage ./internal/web -v`; убедиться, что проверки отсутствующего поведения проваливаются. Если каркас ещё не собирается, после его появления повторить тесты до изменения реализации, чтобы видеть поведенческий провал.
- [x] **Step 3:** Реализовать конфигурацию (`KB_DATA_DIR`, `KB_PUBLIC_URL`, `KB_LISTEN_ADDR`, явный dev-режим), владение блокировкой экземпляра, подключения/миграции, базовые таблицы из раздела 7 спецификации и FTS5. Все ошибки старта возвращать до запуска рабочих маршрутов. `serve` и `check` добавить в CLI.
- [x] **Step 4:** Зафиксировать актуальные совместимые версии зависимостей и sqlc. Настроить `make generate`, `make test`, `make build`; исходные SQL и сгенерированные файлы коммитить вместе. Реализовать сигналы и graceful shutdown с 15 секундами, без idle-таймеров.
- [x] **Step 5:** Выполнить `make generate && go test ./internal/config ./internal/storage ./internal/web -v && go vet ./...`. На временном каталоге дважды запустить/остановить сервер, проверить readiness и отказ второго экземпляра. Ожидается сохранение данных, успешный первый и неуспешный второй одновременный экземпляр.
- [x] **Step 6:** Commit `feat: add Go server and transactional SQLite foundation`.

### Task 2: Пользователи, вход и административная выдача записей

**Files:** Create `internal/auth/{service,password,sessions,csrf,limits}.go`, `internal/web/{auth,admin_users,middleware}.go`, `internal/ui/templates/{base,login,password,admin_users}.html`, `internal/storage/queries/auth.sql`; modify CLI and domain; tests `internal/auth/service_test.go`, `internal/web/auth_test.go`.

**Interfaces:** Consumes `storage.Store`. Produces `auth.New(store *storage.Store, cfg auth.Config) (*auth.Service, error)`; `CreateUser(ctx, actorID domain.ID, in auth.NewUser) (domain.User, error)`, `Login(ctx, login, password string) (auth.LoginResult, error)`, `ResolveSession(ctx, rawToken string) (domain.User, error)`, `ChangePassword(ctx, userID domain.ID, oldPassword, newPassword string) error`, `ResetPassword(ctx, adminID, userID domain.ID, temporaryPassword string) error`, `SetDisabled(ctx, adminID, userID domain.ID, disabled bool) error`, `Logout(ctx, rawToken string) error`. `NewUser` содержит login/name/temporaryPassword/role; `LoginResult` — rawToken и mustChangePassword. Bootstrap CLI — отдельный путь только для первоначального администратора с блокировкой экземпляра.

- [x] **Step 1:** Написать `TestTemporaryPasswordRequiresChange`, `TestNormalRestartKeepsSession`, `TestDisableAndResetRevokeSessions`, `TestOnlyAdminCreatesUsers`, `TestConcurrentDuplicateLogin`, `TestLoginLimitPersistsAfterRestart`, `TestPasswordLengthNotTruncated`. Блокировка после 5 неверных паролей сохраняется 10 минут; для времени использовать управляемые часы, не реальные sleep.
- [x] **Step 2:** Запустить `go test ./internal/auth ./internal/web -run 'Test(Temporary|NormalRestart|Disable|OnlyAdmin|ConcurrentDuplicate|LoginLimit|Password)' -v`, проверить провал ожидаемого поведения.
- [x] **Step 3:** Реализовать нормализацию login, bcrypt/сессии/ограничения и атомарный отзыв сессий. Добавить CLI `kb admin create --login ... --name ... --password-stdin`, не выводящий пароль. При существующем админе CLI не обходит серверное управление. Persisted CSRF-ключ создавать безопасно один раз на томе.
- [x] **Step 4:** Реализовать login/logout/смену пароля/административные формы, CSRF и Origin, безопасную cookie и локальный `return_to`. Написать `TestRejectsExternalReturnTo`, `TestUnsafeRequestsRequireCSRF`, `TestContentDeniedBeforePasswordChange`, `TestForgedProxyHeadersIgnored`, проверяющие реальные ответы/доступ. Контентные обработчики используют одного middleware активной сессии.
- [x] **Step 5:** Выполнить `go test -race ./internal/auth ./internal/web`. Ожидается полный проход тестов, включая запрос со старой cookie после блокировки и защиту прямых маршрутов.
- [x] **Step 6:** Commit `feat: add administrator-issued accounts and persistent sessions`.

### Task 3: Категории, карточки, меню и постоянные ссылки

**Files:** Create `internal/categories/service.go`, `internal/articles/read.go`, `internal/web/{articles,categories}.go`, `internal/ui/templates/{home,sidebar,article,admin_categories}.html`, `web/src/app.js`, `web/src/styles.css`, `package.json`, `package-lock.json`, `vite.config.js`, запросы `categories.sql`, `articles.sql`; tests `internal/articles/read_test.go`, `internal/web/articles_test.go`.

**Interfaces:** Consumes auth и storage. Produces `categories.Service.List(ctx) ([]domain.Category, error)`, `Save(ctx, adminID domain.ID, in categories.Update) (domain.Category, error)`, `Delete(ctx, adminID, categoryID domain.ID, expected domain.Version) error`; `articles.Service.Get(ctx, actorID, articleID domain.ID) (domain.Article, error)`, `List(ctx, actorID domain.ID, categoryID *domain.ID, page int) (articles.Page, error)`, `ListCategoryMenu(ctx, actorID, categoryID domain.ID, cursor string) (articles.MenuPage, error)`. Category/Article содержат ID, title, version; Article также автора, редактора, document/plainText, категорию и даты. `Update` содержит ID/title/order/expectedVersion.

- [x] **Step 1:** Написать `TestRenameKeepsArticleURL`, `TestCardsExcludeUnpublishedAndArchived`, `TestCategoryFilterAndStablePagination`, `TestSidebarPagesBy50`, `TestDeletingNonemptyCategoryFails`, `TestOnlyAdminManagesCategories`. Чтение проверять на fixture опубликованной статьи в настоящей временной SQLite.
- [x] **Step 2:** Запустить `go test ./internal/articles ./internal/categories ./internal/web -v`, увидеть провал новых сценариев.
- [x] **Step 3:** Реализовать GET `/`, `/articles/{uuid}`, меню и фильтр; категории — через административные формы с проверкой версии. По 20 карточек, безопасные фрагменты, UUID не меняется от title. Разделить полную страницу и HTMX-фрагменты, чтобы прямой переход возвращал полноценный документ.
- [x] **Step 4:** Настроить Vite/Tailwind/HTMX, локальную поставку без CDN, меню слева и карточки на главной. Работу с клавиатурой, подписи форм и узкий экран включить в шаблоны. Результат сборки встроить через `go:embed`; `make build` сначала собирает frontend.
- [x] **Step 5:** Выполнить `npm ci && npm run build && go test ./internal/articles ./internal/categories ./internal/web && make build`. Ожидаются работающая карточка/статья по прямой ссылке, недоступность гостю и собранный сервер без Node.js в runtime.
- [x] **Step 6:** Commit `feat: add categories and article browsing interface`.

### Task 4: Личные черновики, визуальный текст и конкурентная публикация

**Files:** Create `internal/articles/{drafts,publish,archive}.go`, `internal/content/{document,render}.go`, `internal/web/drafts.go`, `internal/ui/templates/{drafts,editor,conflict}.html`, `web/src/editor.js`, `playwright.config.ts`, `tests/e2e/conflicts.spec.ts`, запросы `drafts.sql`, `publish.sql`; modify package.json/package-lock.json; tests `internal/articles/publish_test.go`, `internal/content/render_test.go`, `internal/web/drafts_test.go`.

**Interfaces:** Consumes read/auth/store. Produces `content.ValidateAndRender(doc json.RawMessage) (content.Rendered, error)` (`HTML`, `PlainText`); `articles.Service.CreateDraft(ctx, actorID domain.ID, articleID *domain.ID) (domain.Draft, error)`; `SaveDraft(ctx, in articles.SaveDraftInput) (domain.Draft, error)`; `Publish(ctx, actorID, draftID domain.ID, expectedDraft, expectedArticle domain.Version) (domain.Article, error)`; `SetArchived(ctx, adminID, articleID domain.ID, expected domain.Version, archived bool) error`. `Draft` содержит ID/articleID/ownerID/title/categoryID/document/version/baseArticleVersion; `SaveDraftInput` — draftID/actorID/expectedVersion/title/categoryID/document.

- [x] **Step 1:** Написать `TestOnlyOwnerReadsDraft`, `TestPublishedTextStaysUntilRepublish`, `TestConcurrentPublishExactlyOneWinner`, `TestStaleRetryStillConflicts`, `TestSameUserTwoTabsConflict`, `TestOriginalAuthorAndLastEditorDiffer`, `TestArchiveCanBeRestoredByAdmin`. В конкурентном тесте синхронно отпустить барьер для 5 публикаций одной версии; ожидать 1 успех, 4 `ErrConflict`, ровно одно увеличение версии и согласованный текст/индекс.
- [x] **Step 2:** Запустить `go test ./internal/articles ./internal/content ./internal/web -v` и зафиксировать провал новых сценариев.
- [x] **Step 3:** Реализовать серверные версии/короткие транзакции и типизированный конфликт. JSON проверять по allowlist; HTML/обычный текст строить на сервере. Добавить `TestRejectsScriptURLAndUnknownNodes`, `TestDocumentLimits`, `TestRenderingEscapesUserText` для XSS, предельных размеров/глубины/узлов.
- [x] **Step 4:** Подключить Tiptap: текст, заголовки, списки, форматирование, цитаты и ссылки. Явные save/publish отправляют JSON с expected versions. `409` не переинициализирует редактор и не очищает введённое; экран показывает актуальную статью отдельно. Добавить список своих черновиков и предупреждение о несохранённом тексте. Настроить Playwright и `npm run test:e2e`: test-server работает с отдельной временной БД, пользователи создаются только fixture. До таблиц/картинок редактор уже завершённо работает с текстом.
- [x] **Step 5:** Выполнить `npm run build && go test -race ./internal/articles ./internal/content ./internal/web && npm run test:e2e -- conflicts.spec.ts`. В браузерном тесте открыть статью двумя сессиями, опубликовать первую, проверить конфликт второй и сохранение введённого текста.
- [x] **Step 6:** Commit `feat: add private drafts and conflict-safe publication`.

### Task 5: Изображения и таблицы в редакторе

**Files:** Create `internal/media/{service,validation,access,maintenance}.go`, `internal/web/media.go`, `internal/storage/queries/media.sql`; modify content renderer, publish transaction, editor and CLI; tests `internal/media/service_test.go`, `internal/content/table_test.go`, `tests/e2e/editor.spec.ts`; use Playwright setup from Task 4.

**Interfaces:** Consumes auth, drafts и content. Produces `media.Service.Upload(ctx, actorID, draftID domain.ID, source io.Reader, filename string) (domain.Media, error)`, `Open(ctx, actorID, mediaID domain.ID) (media.File, error)`, `CollectOrphans(ctx, olderThan time.Time) (int, error)`. `domain.Media` — ID/type/size/width/height; `media.File` — read-closer и проверенные метаданные. Расширяет валидатор image/table nodes; publish проверяет и атомарно сохраняет ссылки на media.

- [x] **Step 1:** Написать `TestPrivateDraftMediaDeniedToOtherUser`, `TestPublishedMediaRequiresLogin`, `TestCannotPublishSomeoneElsesPrivateImage`, `TestMimeSpoofAndTraversalRejected`, `TestSizeAndPixelLimits`, `TestThirdParallelUploadRejected`, `TestFailedUploadLeavesNoPublishedReference`, `TestGCRespectsDraftAndArticleReferences`.
- [x] **Step 2:** Запустить `go test ./internal/media ./internal/content ./internal/articles -v`, убедиться в провале новых ожиданий.
- [x] **Step 3:** Реализовать файловый staging и безопасные сгенерированные имена, ограничения загрузки, доступ по привязкам и координируемую блокировкой очистку. Не принимать file path/src/владельца из запроса как доверенные данные; физический файл становится доступен только через авторизованный обработчик.
- [x] **Step 4:** Подключить Tiptap image/table, вставку/перетаскивание и изменение структуры таблицы. Расширить server renderer для таблиц и изображений; произвольный HTML, стили и внешние image URL остаются запрещены. Тестировать сохранение/повторное открытие, не только факт наличия toolbar.
- [x] **Step 5:** Выполнить `go test -race ./internal/media ./internal/content ./internal/articles` и `npm run test:e2e -- editor.spec.ts`. Ожидаются опубликованные изображение/таблица после reopen и недоступность личного медиа второму пользователю. Playwright использует одноразовый test-server и временный data-dir без демонстрационных пользователей в обычном запуске.
- [x] **Step 6:** Commit `feat: add protected article images and editable tables`.

### Task 6: Поиск по опубликованному содержимому

**Files:** Create `internal/search/{query,service}.go`, `internal/storage/queries/search.sql`, `internal/web/search.go`, `internal/ui/templates/search.html`; tests `internal/search/service_test.go`; modify транзакции publish/archive и навигацию.

**Interfaces:** Consumes опубликованные `articles` и производный `PlainText` от content. Produces `search.Service.Find(ctx, actorID domain.ID, query string, page int) (search.Page, error)`; `Page` содержит items с articleID/title/snippet/category/author и параметры пагинации. Обновление индекса не является отдельной eventual-consistency задачей: оно входит в транзакцию публикации/архивирования.

- [ ] **Step 1:** Написать `TestFindsCyrillicAndTableText`, `TestTitleRanksAboveBody`, `TestSearchNeverShowsDrafts`, `TestArchiveAndRepublishUpdateIndex`, `TestQuotesPunctuationAndEmptyQuery`, `TestSnippetDoesNotExecuteHTML`, `TestPublishRollbackKeepsIndexConsistent`.
- [ ] **Step 2:** Выполнить `go test ./internal/search ./internal/articles -v`, проверить содержательные провалы новых тестов.
- [ ] **Step 3:** Реализовать `unicode61`, параметризованный запрос с безопасным преобразованием текстового ввода в FTS и ранжирование заголовка выше тела. Пустой запрос направить к обычному списку статей. Реализовать GET `/search`, HTMX-выдачу и полную страницу по прямому URL.
- [ ] **Step 4:** Выполнить `go test -race ./internal/search ./internal/articles ./internal/web`, а затем браузерный сценарий публикация → поиск → архивирование → отсутствие в выдаче.
- [ ] **Step 5:** Commit `feat: add transactional full-text article search`.

### Task 7: Согласованные копии, восстановление и обслуживание

**Files:** Create `internal/backup/{backup,restore,manifest,retention}.go`, `internal/storage/file_locks.go`, `internal/backup/backup_test.go`, `docs/operations.md`; modify media GC and CLI.

**Interfaces:** Consumes storage, immutable media и их блокировку. Produces `backup.Create(ctx, dataDir, backupDir string) (backup.Manifest, error)`, `backup.Restore(ctx, bundleDir, targetDataDir string) error`, `backup.Rotate(ctx, backupDir string, keep int) error`. `Manifest` содержит версию формата, время, список файлов и SHA-256. `restore` требует эксклюзивную блокировку экземпляра; `backup` и GC согласуют файловую блокировку, не останавливая публикацию.

- [ ] **Step 1:** Написать `TestBackupIncludesCommittedWAL`, `TestBackupDuringPublishAndGCIsRestorable`, `TestRestoreRejectsRunningApp`, `TestRestoreRejectsNonemptyTarget`, `TestFailedCopyKeepsPreviousBackups`, `TestRotateKeeps14AfterSuccessfulCopy`, `TestRestoreRevokesSessions`, `TestChecksumMismatchRejectsBeforeReplacement`.
- [ ] **Step 2:** Выполнить `go test ./internal/backup -v`, увидеть провал новых сценариев. Для ошибок записи использовать управляемый failing-writer, не заполнение всего диска машины.
- [ ] **Step 3:** Реализовать снимок SQLite штатным механизмом, неизменяемые файлы, временный комплект и атомарную финализацию manifest. Блокировка исключает GC во время копии; публикация может продолжаться. Запрещено считать простой `cp` живого `.db` корректной копией. В restore проверить целостность, исключить обход пути из manifest и отозвать старые сессии.
- [ ] **Step 4:** Реализовать CLI backup/restore/maintenance, проверку повторных запусков и ротацию только после успешного комплекта. В operations описать ежедневный вызов через host cron/systemd timer и перенос копий за пределы сервера как отдельную инфраструктурную обязанность.
- [ ] **Step 5:** Выполнить `go test -race ./internal/backup ./internal/media`. На временных каталогах провести полный цикл: пользователь/черновик/статья/картинка → backup → restore в другую папку → проверка чтения и входа новой сессией. Старый токен должен быть недействителен.
- [ ] **Step 6:** Commit `feat: add consistent backup and restore operations`.

### Task 8: Docker Compose и подключение существующего Nginx

**Files:** Create `Dockerfile`, `.dockerignore`, `deploy/{compose.yaml,.env.example,nginx-compose.fragment.yaml,nginx-site.conf.example}`, `tests/integration/{compose.kb.yaml,compose.proxy.yaml,nginx.conf,test_proxy.py}`; update operations и README.

**Interfaces:** Consumes готовый `kb serve/check/backup`, встроенные ресурсы и том `/data`. Produces один runtime-образ и независимый Compose-проект; наружный контракт — `knowledgebase-app:8080` в сети `knowledgebase-ingress`. Конфигурация примера использует фактический `KB_PUBLIC_URL` и документирует доверенную proxy-сеть, не доверие всему Интернету.

- [ ] **Step 1:** В disposable-тестах описать два разных Compose-проекта с общей external network. В proxy-проекте есть существующий тестовый клиентский маршрут; он должен работать до и после подключения базы. Написать `test_connect_and_recreate`: вход/статья через proxy, принудительное пересоздание только приложения, восстановление через Docker DNS без рестарта proxy после TTL и готовности. До появления конфигурации тест обязан провалиться.
- [ ] **Step 2:** Реализовать multi-stage сборку с `npm ci`, frontend build и Go без CGO. Runtime — non-root, без компиляторов, с read-only root, ограниченным tmpfs, отдельным `/data` и каталогом копий. Проставить лимиты из Global Constraints, healthcheck через `kb check`, grace 30 секунд, restart unless-stopped; не добавлять Nginx в production Compose базы.
- [ ] **Step 3:** Подготовить фрагмент Compose существующего Nginx: сохранить его старые сети и добавить external `knowledgebase-ingress`. В конфиге маршрута использовать Docker resolver `127.0.0.11 valid=10s` и динамическое имя `knowledgebase-app:8080`, не `localhost`. Проверить отсутствие публикации host 80/443 у базы. Примеры не заменяют чужие конфиги целиком и не угадывают имена сервисов.
- [ ] **Step 4:** Документировать два способа подключения текущего контейнера Nginx: live `docker network connect` плюс сохранение сети в Compose; либо пересоздание только Nginx с `--no-deps` в согласованное окно. Отличать это от `nginx -t`/reload HTTP-конфига. Не запускать команды на реальных клиентских сервисах в рамках реализации репозитория.
- [ ] **Step 5:** Выполнить `docker compose -f deploy/compose.yaml config --quiet`, `docker build -t knowledgebase:test .`, затем `python3 tests/integration/test_proxy.py`. Скрипт сам создаёт уникальные test project/network/data, запускает оба стека, проверяет HTTP и неизменность тестового клиентского маршрута, чистит только свои ресурсы. Ожидаются успешные запросы после смены IP, отказ второго app над тем же томом и сохранение данных при restart.
- [ ] **Step 6:** Commit `build: add isolated Compose deployment and shared Nginx integration`.

### Task 9: Сквозная приёмка, память и воспроизводимая проверка

**Files:** Create `tests/e2e/{accounts,articles,search}.spec.ts`, `tests/load/{seed,run,report}.go`, `.github/workflows/ci.yml`, `docs/validation.md`; update `tests/e2e/conflicts.spec.ts`, Makefile, package scripts, operations/README.

**Interfaces:** Consumes все готовые компоненты. Produces `make verify` (generate/build/vet/race/browser), `make test-integration` и `make test-capacity`. Load runner создаёт только test data, проверяет статусы и финальные версии, сохраняет новые timestamped результаты, возвращает ненулевой код при нарушении.

- [ ] **Step 1:** Добавить браузерные сценарии по приёмке 1–7 спецификации: два независимых пользователя, приватный черновик/медиа, временный пароль, две вкладки, конфликт с сохранённым текстом, стабильный URL, опубликованная таблица/картинка, поиск и архив. Запустить их до исправлений интеграции и зафиксировать реальные неудачные сценарии, если они есть; не добавлять искусственный провал уже работающих функций.
- [ ] **Step 2:** Реализовать `make verify`: генерация SQL/ресурсов, `go vet ./...`, `go test -race ./...`, browser tests. CI использует временную настоящую SQLite и проверяет отсутствие diff после генерации, без реальных корпоративных данных и secrets. Native Go-код для race проверяется поддерживаемым toolchain; runtime-образ остаётся без CGO.
- [ ] **Step 3:** Реализовать capacity runner: 5 000 статей, 50 конкурентных читателей и 5 авторов, 10 минут, плюс две загрузки по допустимому пределу и одна штатная резервная копия внутри того же лимита памяти. Он проверяет ответы и согласованность версий; тестовый admin создаёт данные отдельно от production. Снимать cgroup peak/current, OOM events, размер набора, время старта/readiness; для shared Nginx записывать baseline и добавочный пик при изолированной нагрузке базы. Ограничения к контейнеру действительно применены, а не только присутствуют в YAML.
- [ ] **Step 4:** Выполнить `make verify && make test-integration && make test-capacity`. Не считать ограниченный короткий прогон эквивалентом полных 10 минут. Отдельно проверить SIGTERM во время запроса, SIGKILL после committed-записи и reopen БД. Задокументировать реальные passed/failed/skipped и параметры машины; если доступ к целевому серверу отсутствует, локальный результат не выдавать за серверную приёмку.
- [ ] **Step 5:** Завершить README/operations: запуск из отдельной папки, подготовка прав тома, bootstrap админа через stdin, nginx-подключение, HTTPS/домен, обновление с backup, restore, обычный restart. Повторить установку по этим инструкциям в пустом test data-dir. Привести измерения в `docs/validation.md`; 1 ГБ не объявлять доказанным по одному heap-профилю или отсутствию падения.
- [ ] **Step 6:** Commit `test: verify knowledge base workflows concurrency and memory budget`.

## Проверка полноты и порядок выполнения

| Требования спецификации | Задачи |
| --- | --- |
| Архитектура Go/SQLite и постоянный сервер | 1, 8 |
| Учётные записи, права, сессии | 2 |
| Категории, карточки, авторство, прямые ссылки | 3, 4 |
| Визуальный редактор и личные черновики | 4, 5 |
| Конфликты и целостность | 1, 2, 4, 9 |
| Изображения и таблицы | 5 |
| Поиск | 6 |
| Копии, восстановление, файловая очистка | 5, 7 |
| Свой Compose, существующий Nginx, DNS | 8 |
| Общая память, сквозная приёмка, повторяемость | 8, 9 |

Задачи выполняются последовательно: контракты записи, авторизации, media и публикации тесно связаны. Для этого проекта рекомендуемая реализация — исполнителем в текущей сессии по `superpowers:executing-plans`, без одновременного редактирования общих контрактов разными агентами. Этот план не запускает реализацию и не публикует приложение.
