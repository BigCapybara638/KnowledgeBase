# Деплой wiki.axentris.ru

Knowledgebase работает одним контейнером с SQLite в общей сборке `/var/www/axentris_core`. В `/var/www/knowledgebase` находятся `docker-compose.yml`, `docker-compose.override.yml` с тегом образа, `.env`, `data/` и `backups/`. Сборка выполняется в GitHub Actions; готовый Docker-образ передаётся архивом по SSH с проверкой SHA-256. Registry не требуется.

## Первая установка

Сначала опубликуйте конфигурацию core: include обоих файлов Knowledgebase, сеть `knowledgebase-web`, виртуальный хост `nginxs/conf.d/knowledgebase.conf` и `wiki.axentris.ru` в `certbot/domains.txt`. DNS A/AAAA wiki должен указывать на VPS; порты 80/443 принимает основной nginx. Core workflow сначала синхронизирует файлы, а при отсутствии манифестов Knowledgebase откладывает перезапуск до первого деплоя приложения.

На VPS под root подготовьте каталоги для SSH-пользователя `deploy`:

```bash
install -d -m 0750 -o deploy -g deploy /var/www/knowledgebase
install -d -m 2750 -o 65532 -g deploy /var/www/knowledgebase/data /var/www/knowledgebase/backups
if [ ! -f /var/www/knowledgebase/.env ]; then
  printf 'KB_PUBLIC_URL=https://wiki.axentris.ru\n' > /var/www/knowledgebase/.env
fi
chown deploy:deploy /var/www/knowledgebase/.env
chmod 600 /var/www/knowledgebase/.env
```

`deploy` должен иметь доступ к Docker и каталогу `/var/www/axentris_core`; `.env` core уже содержит `LETSENCRYPT_EMAIL`. UID 65532 принадлежит приложению; группа deploy и setgid на каталогах позволяют проверять наличие базы без запуска приложения от root.

В GitHub Secrets репозитория нужны `VPS_HOST`, `VPS_USER=deploy`, `VPS_SSH_KEY`. Необязательный `VPS_PORT` по умолчанию 22. Push в `main` запускает проверки и деплой; повторный запуск доступен через Actions → Verify and deploy wiki → Run workflow. SSH использует временный known_hosts и accept-new в рамках одного job.

Первый деплой загружает образ и манифесты, подключает основной nginx к сети и запускает выпуск отдельного сертификата wiki через общий Certbot. При пересоздании nginx будет короткий перерыв входящего трафика. Приложение остаётся выключенным до создания администратора: об этом сообщает `.bootstrap-required`. Профиль `knowledgebase` защищает от случайного запуска общим `docker compose up` во время bootstrap.

После успешного первого workflow выполните на VPS в Bash под deploy:

```bash
cd /var/www/axentris_core
read -r -s -p 'Временный пароль администратора: ' kb_bootstrap_password
printf '\n'
printf '%s\n' "$kb_bootstrap_password" | docker compose run --rm --no-deps -T knowledgebase-app admin create --login admin --name 'Администратор' --password-stdin
kb_bootstrap_status=$?
unset kb_bootstrap_password
if [ "$kb_bootstrap_status" -eq 0 ]; then
  docker compose up -d --no-deps --wait --wait-timeout 120 knowledgebase-app &&
    docker compose exec -T knowledgebase-app /app/kb check &&
    rm -- /var/www/knowledgebase/.bootstrap-required
fi
```

Не удаляйте маркер при ошибке bootstrap. Если администратор уже создан, повторно его не создавайте: выполните только `up`, `check` и после успешной проверки удалите маркер. Пароль не записывается в командную строку или `.env`; после первого входа его нужно сменить.

```bash
curl -fsS https://wiki.axentris.ru/readyz
docker compose logs --tail=100 knowledgebase-app nginx certbot
```

HTTP challenge проходит через основной nginx. Certbot выпускает отдельный сертификат wiki и автоматически продлевает его; watcher nginx подхватывает сертификаты. До выпуска TLS для wiki отклоняется. Отдельный nginx в проекте Knowledgebase не нужен; все пути, включая `/mcp`, проксируются на `knowledgebase-app:8080`.

## Последующие обновления

Каждый push в main проходит Go/UI/deploy/integration проверки, затем собирает образ `knowledgebase:<полный SHA>`. После проверки архива и загрузки образа скрипт делает согласованный backup SQLite старой версией приложения. Только после успешного backup заменяются манифесты. Контейнер обновляется через core Compose с ожиданием готовности и `/app/kb check`.

`.env`, данные и резервные копии не перезаписываются. Предыдущие манифесты сохраняются как `docker-compose.previous.yml` и `docker-compose.previous.override.yml`. При ошибке готовности автоматического отката нет: новая версия могла применить миграции. Восстановление описано в [operations.md](operations.md#восстановление).

Команды обслуживания выполняются из `/var/www/axentris_core`, чтобы использовать тот же Compose-проект и сеть:

```bash
cd /var/www/axentris_core
docker compose ps --all
docker compose logs --tail=100 knowledgebase-app
docker compose exec -T knowledgebase-app /app/kb backup
```

Для запуска всей сборки с уже настроенным Knowledgebase используйте `docker compose --profile knowledgebase up -d`. Не запускайте второй экземпляр приложения из проектного каталога: SQLite должна обслуживаться одним процессом.

Блокировка `/var/www/axentris_core/.deploy.lock` согласует перезапуски core и wiki; удалять этот файл не нужно. Пустой каталог `/var/www/knowledgebase/.deploy-lock` после аварийного завершения можно удалить через `rmdir`, предварительно убедившись, что деплой не выполняется. Успешно загруженные временные файлы удаляются, неудачные остаются в `incoming-*` для диагностики. Скрипт не выполняет `down`, `prune` или удаление томов.
