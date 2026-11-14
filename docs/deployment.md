# Автодеплой wiki.axentris.ru

В репозитории один workflow: `.github/workflows/deploy.yml`. Pull request запускает проверки. Push в `main` после проверок собирает образ `knowledgebase:<полный-SHA-коммита>`, сохраняет его через `docker image save` в gzip-архив и передаёт по SSH на VPS. Сервер проверяет SHA-256, выполняет `docker image load` и обновляет только базу знаний. Registry, GitHub Packages и Docker login не используются.

На VPS не нужны исходники, Git, Go или Node.js. Требуются Linux **amd64**, Bash, Docker Engine и Compose **v2.20+**. Nginx остаётся в проекте `/var/www/axentris`, единолично публикует 80/443 и направляет `wiki.axentris.ru` на `knowledgebase-app:8080` по сети `knowledgebase-ingress`.

## 1. Один раз подготовить VPS

Следующие команды выполняются от root. Для простоты `VPS_USER` ниже — `root`; его SSH-ключ должен быть разрешён на сервере. Для отдельного пользователя понадобятся права Docker и доступ к каталогу установки, включая чтение `data/knowledgebase.db` для определения первого запуска.

```bash
install -d -m 0750 /opt/knowledgebase
install -d -m 0700 -o 65532 -g 65532 /opt/knowledgebase/data /opt/knowledgebase/backups
docker network inspect knowledgebase-ingress >/dev/null 2>&1 || docker network create knowledgebase-ingress
docker compose version
```

Создай `/opt/knowledgebase/.env` **один раз**, например через `nano /opt/knowledgebase/.env`:

```dotenv
KB_PUBLIC_URL=https://wiki.axentris.ru
KB_NETWORK_NAME=knowledgebase-ingress
```

```bash
chmod 600 /opt/knowledgebase/.env
```

Если `.env` уже существует, сохрани его и отредактируй только нужные настройки. Workflow его **не загружает с GitHub, не удаляет, не перезаписывает и не дописывает**. Тег образа хранится отдельно в `compose.override.yaml`, который автоматически учитывает обычная команда `docker compose` из каталога установки. Даже старый `KB_IMAGE` в `.env` не мешает: образ в override имеет приоритет.

Вход в Docker registry и токены GitHub Packages не нужны. В override установлено `pull_policy: never`: Compose использует загруженный образ и не пытается скачать его из Docker Hub.

## 2. Настроить GitHub Actions

В **Settings → Secrets and variables → Actions → Repository secrets** добавь:

| Secret | Значение |
|---|---|
| `VPS_HOST` | IPv4 или DNS-имя VPS, без `https://` |
| `VPS_USER` | SSH-пользователь, например `root` |
| `VPS_SSH_KEY` | Полный приватный SSH-ключ для входа на VPS |
| `VPS_PORT` | SSH-порт; необязательно, по умолчанию `22` |

Публичную часть SSH-ключа добавь в `authorized_keys` нужного пользователя. Приватный ключ должен позволять неинтерактивный вход, без запроса passphrase.

Отдельный secret с ключом сервера не нужен. SSH использует `StrictHostKeyChecking=accept-new`: при первом подключении каждого запуска Actions автоматически доверяет полученному ключу и сохраняет его во временном `known_hosts`. Все последующие SSH/SCP-подключения этого запуска проверяют тот же ключ. Между разными запусками Actions ключ не сохраняется, поэтому его изменение относительно предыдущего деплоя автоматически не обнаруживается.

Сборка архива и деплой выполняются только для push в `main`; после настройки отправь изменения в эту ветку. Одновременные запуски сериализуются. Текущий образ помечается SHA, тег `latest` не используется.

## 3. Первый деплой и администратор

Первый успешный workflow передаст и загрузит образ, затем установит:

```text
/opt/knowledgebase/
  .env                      # ручные настройки, сохраняются
  .env.example              # обновляемый образец
  compose.yaml              # обновляемая конфигурация сервиса
  compose.override.yaml     # выбранный образ с тегом SHA
  data/                     # SQLite, медиа, ключи
  backups/                  # резервные копии
```

На пустом `data` workflow создаёт `.bootstrap-required`, выводит `Image and configuration prepared` и **не запускает HTTP**. Маркер сохраняется между деплоями, даже если неудачная попытка создания администратора уже создала SQLite-файл. Создай первого администратора на VPS; только после успешного создания удали маркер и запусти приложение. Пароль должен содержать минимум 8 символов и занимать не более 72 байт UTF-8:

```bash
cd /opt/knowledgebase
read -r -s -p 'Временный пароль администратора: ' kb_bootstrap_password
printf '\n'
if printf '%s\n' "$kb_bootstrap_password" | docker compose run --rm --no-deps -T knowledgebase-app admin create --login admin --name 'Администратор' --password-stdin; then
  rm -f -- .bootstrap-required
  docker compose up -d --wait --wait-timeout 120 && docker compose exec -T knowledgebase-app /app/kb check
fi
unset kb_bootstrap_password
```

При повторных деплоях администратора создавать не нужно. Если создание администратора прервалось, исправь причину и заверши его до запуска HTTP. Если администратор уже успешно создан, но процесс оборвался до удаления маркера, удали `.bootstrap-required` вручную и запусти сервис. Не удаляй маркер после неуспешного создания администратора.

## 4. Подключить существующий Nginx Axentris

Создай DNS-запись `A` для `wiki.axentris.ru` на IPv4 VPS. Если используешь `AAAA`, она тоже должна вести на этот VPS с настроенным IPv6. Порты 80/443 уже обслуживает текущий Nginx.

В `/var/www/axentris` дополни **существующий** Compose. У сервиса `nginx` сохрани `build`, `ports`, текущие SSL/log volumes и `depends_on`; добавь сеть и mount каталога:

```yaml
services:
  nginx:
    # ... существующие настройки ...
    volumes:
      - /opt/ssl/certbot/www:/var/www/certbot
      - /opt/ssl/certbot/conf:/etc/letsencrypt
      - /opt/nginx/logs:/var/log/nginx
      - ./nginx-vhosts:/etc/nginx/vhosts:ro
    networks:
      - app-network
      - knowledgebase-ingress

networks:
  app-network:
    # Сохрани текущие настройки этой сети, если они уже заданы.
  knowledgebase-ingress:
    external: true
    name: knowledgebase-ingress
```

Не подключай PostgreSQL и `app` к сети базы знаний. Их настройки остаются прежними. Не меняй имя Compose-проекта Axentris или имя его PostgreSQL-тома.

В исходном `nginx.conf`, который Dockerfile копирует в `/etc/nginx/nginx.conf`, добавь **внутрь `http {}`**, вне `server {}`:

```nginx
include /etc/nginx/vhosts/*.conf;
```

Существующие блоки `corp.axentris.ru` сохраняются. В присланном конфиге внутри `/api/` повторяются `proxy_buffering`, `proxy_request_buffering` и `proxy_cache`: оставь каждую директиву один раз.

Сначала подготовь **только HTTP** для получения сертификата:

```bash
mkdir -p /var/www/axentris/nginx-vhosts
```

Создай `/var/www/axentris/nginx-vhosts/wiki.conf`:

```nginx
server {
    listen 80;
    server_name wiki.axentris.ru;
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }
    location / {
        return 301 https://$host$request_uri;
    }
}
```

Проверь версию: `docker exec nginx nginx -v`. Для динамического upstream из примера требуется **Nginx 1.27.3+**; при необходимости обнови Nginx-стадию Dockerfile перед сборкой.

Из каталога Axentris проверь Compose и пересобери **только Nginx** для применения include, mount и сети. При пересоздании будет короткий перерыв для `corp`:

```bash
cd /var/www/axentris
docker compose config --quiet
docker compose up -d --build --no-deps nginx
docker exec nginx nginx -t
```

Теперь выпусти сертификат через существующий Certbot либо отдельный контейнер с теми же каталогами:

```bash
docker run --rm -it \
  -v /opt/ssl/certbot/www:/var/www/certbot \
  -v /opt/ssl/certbot/conf:/etc/letsencrypt \
  certbot/certbot certonly --webroot -w /var/www/certbot -d wiki.axentris.ru
```

После появления сертификата замени `nginx-vhosts/wiki.conf` полным содержимым [deploy/nginx-wiki.conf.example](../deploy/nginx-wiki.conf.example). Этот файл включает upstream и оба server-блока. **Не добавляй одновременно generic-примеры upstream/site**, иначе получится дублирование.

```bash
docker exec nginx nginx -t && docker exec nginx nginx -s reload
curl -fsS https://wiki.axentris.ru/readyz
```

Nginx направляет все пути wiki, включая `/mcp`, на `knowledgebase-app:8080`; маршруты corp и его API обслуживаются существующими server-блоками. Для обновлений базы Nginx пересобирать не требуется: Docker DNS обновляется через `resolver`/`resolve`.

Добавь сертификат wiki к существующей процедуре автоматического продления. При продлении с Docker нужны те же два volume, команда `certbot/certbot renew` и успешный `nginx -t`/reload после обновления сертификатов. Reload сам по себе сертификаты не продлевает.

## 5. Повторные деплои и ошибки

Каждый push в `main`: проверки → сборка SHA-образа → `docker image save | gzip` → передача архива, SHA-256 и файлов `compose.yaml`, `deploy.sh`, `.env.example` во временный каталог → проверка контрольной суммы и конфигурации → `docker image load` и проверка наличия нужного тега → backup текущей базы **старым** контейнером → обновление конфигурации → `up --wait` и проверка `/readyz` через `/app/kb check`.

Если сервис остановлен, backup выполняется одноразовым контейнером из текущей конфигурации. Ошибка передачи, проверки архива, загрузки образа или backup останавливает деплой до замены файлов. Ошибка запуска/готовности делает Actions красным; автоматического отката образа нет, поскольку новая версия могла применить миграции SQLite. Предыдущие конфигурации сохраняются как `compose.previous.yaml` и `compose.previous.override.yaml`; для восстановления базы используй [инструкцию restore](operations.md#восстановление).

```bash
cd /opt/knowledgebase
docker compose ps
docker compose logs --tail=100 knowledgebase-app
docker compose exec -T knowledgebase-app /app/kb check
```

Workflow не трогает другие проекты, `.env`, данные и резервные копии; не выполняет `down`, `prune` или удаление томов. После успешного запуска временный архив и загруженные файлы удаляются. Загруженные файлы неудачного запуска остаются в `incoming-*` для диагностики; архив может занимать заметное место на диске. Предыдущие образы остаются в Docker для возможного ручного отката; автоматической очистки образов других проектов нет. После аварийного завершения процесса может остаться `.deploy-lock`: убедись, что деплой больше не выполняется, и удали **пустой** каталог командой `rmdir /opt/knowledgebase/.deploy-lock` перед повторным запуском.

Блокировка и имя Compose-проекта предполагают одну установку в `/opt/knowledgebase`. Один экземпляр приложения обслуживает SQLite на локальном диске; несколько реплик над этими данными запускать нельзя.

Справка: [docker image save](https://docs.docker.com/reference/cli/docker/image/save/), [docker image load](https://docs.docker.com/reference/cli/docker/image/load/), [Compose override](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/), [Nginx upstream resolve](https://nginx.org/en/docs/http/ngx_http_upstream_module.html#resolve), [Certbot webroot](https://eff-certbot.readthedocs.io/en/stable/using.html#webroot).
