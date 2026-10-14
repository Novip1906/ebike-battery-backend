# ebike-battery-backend

Бэкенд заявочной системы по теме **«Оценка времени разряда аккумулятора электровелосипеда»**.
Курс «Разработка Интернет Приложений», ИУ5, 2026. Вариант 10 раздела «Транспорт, инфраструктура и строительство».

- **Услуга** — режим работы мотора (Eco, Tour, Sport, Turbo и другие режимы системы Bosch eBike).
- **Заявка** (с третьей лабораторной) — расчёт остатка заряда аккумулятора после поездки по маршруту с заданным перепадом высот.
- **Роли** — велосипедист (`rider`) и инженер сервиса (`bike_service_engineer`).

Ветки: `motor_mode_inMemory` — лабораторная 1 (коллекция в памяти), `motor_mode_postgres` — лабораторная 2 (PostgreSQL и GORM), `motor_modes_api` — лабораторная 3 (веб-сервис REST для SPA), `motor_modes_jwt` — лабораторная 4 (авторизация по JWT, чёрный список в Redis, swagger).

## Стек

Go 1.26, gin, GORM + PostgreSQL 16, Adminer, Minio в Docker (`minio-go/v7`), bcrypt, JWT (`golang-jwt/jwt/v5`), Redis 7 (`go-redis/v9`), swagger (`swaggo/swag`, `gin-swagger`). Веб-сервис отвечает JSON, страницы ЛР1–2 остаются на `html/template` без JavaScript.

## Запуск

```bash
docker compose up -d                 # Minio (9000/9001), PostgreSQL (5434), Adminer (8081), Redis (6379)
./scripts/minio_setup.sh ../media    # создать публичный бакет и залить медиа
cp .env.example .env                 # параметры подключения к БД, Minio и Redis, секрет JWT
./scripts/db_setup.sh --reset        # удалить таблицы, миграция GORM (cmd/migrate) и наполнение db/seed.sql
go run .                             # страницы http://localhost:8080/motor_modes, API http://localhost:8080/api
                                     # swagger http://localhost:8080/swagger/index.html
go test ./...                        # тесты веб-сервиса
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g main.go   # пересобрать docs/ после правки аннотаций
```

Adminer: http://localhost:8081, система PostgreSQL, сервер `postgres`, пользователь `ebike`, пароль `ebike_password`, база `ebike_battery`.
Порт 5434 снаружи выбран, чтобы не пересекаться с локальным PostgreSQL на 5432.

Переменные окружения (`.env`): `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME` — PostgreSQL; `MINIO_PUBLIC_URL` — адрес бакета для ссылок на фото и видео; `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET` — загрузка файлов в Minio; без ключей Minio приложение не запускается; `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` — Redis с чёрным списком JWT, без Redis приложение не запускается; `JWT_SECRET` — секрет подписи HS256 (обязателен), `JWT_EXPIRES_IN` — срок жизни токена, по умолчанию `1h`.

## Авторизация

- **Аутентификация.** `POST /api/riders/login` проверяет пароль по bcrypt-хешу и выдаёт JWT (HS256, `internal/auth/jwt.go`). В токене: `rider_id`, `is_bike_service_engineer`, `sub` — логин, `iat`, `exp`, `iss`.
- **Передача токена.** Клиент отправляет заголовок `Authorization: Bearer <access_token>` в каждом запросе.
- **Проверка.** Middleware в `internal/api/middleware.go`: `RequireRider` стоит на методах создателя и отвечает 401, если токена нет, он подделан, просрочен или отозван; `OptionalRider` стоит на методах чтения — с токеном вычисляет признаки `created_by_current_rider` / `liked_by_current_rider`, без токена или с негодным токеном обрабатывает запрос как гостевой.
- **Деавторизация.** `POST /api/riders/logout` записывает токен в чёрный список Redis (`internal/blacklist`): ключ `ebike_battery.jwt_blacklist.<JWT>`, значение — логин велосипедиста, время жизни ключа — до истечения токена, дальше Redis удаляет его сам. Если Redis недоступен во время работы, методы создателя отвечают 500, а методы чтения обрабатываются как гостевые.
- **Создатель режима** заполняется автоматически из `rider_id` токена, с клиента не передаётся.

| Группа | Методы |
|---|---|
| гость | `GET /api/motor_modes`, `GET /api/motor_modes/feed[/:motor_mode_id]`, `POST /api/riders/register`, `POST /api/riders/login` |
| велосипедист (создатель) | всё, что доступно гостю, и `GET /api/motor_modes/draft`, `POST /api/motor_modes`, `PUT /api/motor_modes/:motor_mode_id/publish`, `DELETE /api/motor_modes/:motor_mode_id`, `POST /api/motor_modes/:motor_mode_id/like`, `POST /api/riders/logout` |

Содержимое Redis: `docker exec -it ebike_redis redis-cli -a ebike_redis_password`, далее `KEYS *`, `GET <ключ>`, `TTL <ключ>`.

Страницы ЛР1–2 работают без входа: на них создателем остаётся велосипедист 1 из функции-singleton `auth.CurrentRiderID()` (`internal/auth/current_rider.go`).

## Swagger

Описание методов генерируется из аннотаций над обработчиками (`swaggo/swag`) в `docs/`: `docs.go`, `swagger.json`, `swagger.yaml`. Интерфейс — http://localhost:8080/swagger/index.html, JSON — http://localhost:8080/swagger/doc.json.
Без авторизации в swagger выполняются только методы чтения, регистрация и вход. Для методов создателя (отмечены замком): выполнить `POST /riders/login`, скопировать `access_token`, нажать **Authorize** и ввести `Bearer <access_token>`.

## База данных

Три таблицы, каскадного удаления нет (все внешние ключи `ON DELETE RESTRICT`):

| Таблица | Назначение | Столбцы |
|---|---|---|
| `riders` | велосипедисты и инженеры сервиса | `id`, `login` varchar(50) unique, `password_hash` varchar(100) — bcrypt, `is_bike_service_engineer` bool |
| `motor_modes` | режимы работы мотора (услуги) | `id`, `mode_name` varchar(50), `short_description` varchar(600) null, `status` varchar(20) (`draft` / `published` / `deleted`), `image_key` varchar(120), `video_key` varchar(120), `support_percent` int null, `consumption_wh_per_km` numeric(5,1) null, `created_at` timestamptz, `creator_rider_id` → `riders.id`, `published_at` timestamptz null |
| `motor_modes_likes` | м-м «велосипедист поставил лайк режиму» | `id`, `rider_id` → `riders.id`, `motor_mode_id` → `motor_modes.id`; unique(`rider_id`, `motor_mode_id`) |

Фото и видео у режима обязательные: `image_key` и `video_key` — `NOT NULL`. Необязательные (null) только поля, которые заполняются при публикации: `short_description`, `support_percent`, `consumption_wh_per_km`, `published_at`.
Частичный уникальный индекс `one_draft_per_rider` на `creator_rider_id where status = 'draft'` гарантирует не более одного черновика у велосипедиста.
Модели — `internal/ds`, миграция — `cmd/migrate/main.go` (`AutoMigrate`), данные — `db/seed.sql` (можно выполнить в Adminer).

## Веб-сервис

Справочник эндпоинтов. Код: маршруты — `internal/api/router.go`, обработчики — `internal/api/motor_modes.go` и `internal/api/riders.go`, JSON запросов и ответов — `internal/serializer/`.

### Общие правила

- Все адреса начинаются с `/api`. Запросы и ответы — JSON (`Content-Type: application/json`), кроме `POST /api/motor_modes` — `multipart/form-data`.
- Текущий велосипедист определяется по JWT из заголовка `Authorization: Bearer <access_token>`. Без токена запрос гостевой: методы чтения работают, методы создателя отвечают 401.
- Системные поля `id`, `status`, `creator_rider_id`, `created_at`, `published_at` клиент изменить не может: в сериализаторах запросов их нет, лишние поля игнорируются.
- Режимы в статусе `deleted` клиенту не отдаются. Переходы статусов: `draft` → `published` (PUT), `draft` / `published` → `deleted` (DELETE), назад в черновик нельзя (`MotorModeStatus.CanChangeTo`, `internal/ds/motor_modes_rules.go`).
- Ошибка: `{"message": "текст"}` и код состояния HTTP — 400, 401 (нет действующего JWT), 403 (чужой режим), 404, 405, 409, 413, 415, 502 или 500. Поля `status` в теле нет, результат передаётся только кодом состояния.
- `image_url` / `video_url` — полный адрес файла в Minio, есть у каждого режима (фото и видео обязательные).

### Режимы работы мотора (`motor_modes`)

#### `GET /api/motor_modes` — список опубликованных режимов

- Обработчик: `ListMotorModes`. Доступ: гость и велосипедист.
- Query: `max_consumption_wh_per_km` — необязательный фильтр по расходу батареи, число больше 0 и меньше 1000. Без фильтра — все опубликованные.
- 200:
  ```json
  {"motor_modes": [{"id": 10, "mode_name": "Sprint", "image_url": "http://localhost:9000/ebike-motor-media/sprint_mode.jpg",
    "support_percent": 280, "consumption_wh_per_km": 12.1, "like_count": 3, "created_by_current_rider": 1}],
   "max_consumption_wh_per_km": 13, "consumption_min": 4.6, "consumption_max": 20}
  ```
  `created_by_current_rider` — 1, если режим создал текущий велосипедист, иначе 0 (у гостя всегда 0). `consumption_min` / `consumption_max` — границы слайдера фильтра.
- Ошибки: 400 — фильтр не число или вне диапазона.

#### `GET /api/motor_modes/feed[/:motor_mode_id][?next=true]` — лента

- Обработчики: `GetMotorModeFeed` (без id) и `GetMotorModeFeedByID`. Доступ: гость и велосипедист. Из БД всегда берётся одна строка.
- Без id — первый опубликованный режим; с id — этот режим; с id и `?next=true` — следующий опубликованный после id (после последнего снова первый; черновики и удалённые пропускаются).
- 200 — режим целиком (`MotorModeDetail`):
  ```json
  {"id": 15, "mode_name": "Walk Assist", "status": "published",
   "liked_by_current_rider": 1, "like_count": 1, "created_by_current_rider": 1, "creator_rider_id": 1,
   "image_url": "…/motor_mode_15.jpg", "video_url": "…/motor_mode_15.mp4", "support_percent": 100,
   "consumption_wh_per_km": 3.4, "range_km": 235, "short_description": "…", "created_at": "…", "published_at": "…"}
  ```
  `liked_by_current_rider` — 1, если текущий велосипедист поставил лайк этому режиму, иначе 0 (у гостя всегда 0). `range_km` — запас хода на аккумуляторе 800 Вт·ч.
- Ошибки: 400 — id не число; 404 — режима нет, он черновик или удалён.

#### `GET /api/motor_modes/draft` — черновик текущего велосипедиста

- Обработчик: `GetMotorModeDraft`. Доступ: велосипедист. id не указывается, владелец берётся из JWT, у велосипедиста не больше одного черновика.
- 200 — `MotorModeDetail` со статусом `draft`. Ошибки: 401 — нет действующего JWT; 404 — черновика нет.

#### `POST /api/motor_modes` — добавить режим с фото и видео

- Обработчик: `CreateMotorMode`. Доступ: велосипедист. Тело — `multipart/form-data`:
  - `mode_name` — текст, обязательно, до 50 символов;
  - `image` — файл JPEG / PNG / WebP до 10 МБ, обязательно;
  - `video` — файл MP4 до 50 МБ, обязательно.
- Создаёт черновик (`status = draft`, создатель — велосипедист из JWT, `created_at` — сейчас). Файлы сохраняются в Minio под латинскими именами `motor_mode_<id>.<расширение>`, имена записываются в `image_key` / `video_key`. Всё в одной транзакции: если Minio недоступен, черновик не создаётся, загруженные файлы удаляются.
- 201 — `MotorModeDetail`, заголовок `Location: /api/motor_modes/draft`.
- Ошибки: 400 — нет названия, нет фото или видео, файл не того типа, `image` / `video` переданы текстом; 401 — нет действующего JWT; 409 — черновик уже есть; 413 — файл больше лимита; 415 — тело не form-data; 502 — Minio недоступен.

#### `PUT /api/motor_modes/:motor_mode_id/publish` — опубликовать черновик

- Обработчик: `PublishMotorMode`. Доступ: велосипедист, только свой черновик. Тело:
  ```json
  {"short_description": "до 600 символов", "support_percent": 100, "consumption_wh_per_km": 3.4, "mode_name": "необязательно"}
  ```
  `support_percent` — целое 0–1000, `consumption_wh_per_km` — больше 0 и меньше 1000, округляется до 0.1.
- Меняет статус `draft` → `published`, `published_at` ставит сервер. 200 — `MotorModeDetail`.
- Ошибки: 400 — поле не заполнено или вне диапазона; 401 — нет действующего JWT; 403 — режим создал другой велосипедист; 404 — режима нет или он удалён; 409 — режим уже опубликован.

#### `DELETE /api/motor_modes/:motor_mode_id` — удалить свой режим

- Обработчик: `DeleteMotorMode`. Доступ: велосипедист, только свой режим. Мягкое удаление через ORM: статус `deleted`, строка остаётся в БД.
- 200: `{"id": 9, "status": "deleted"}`.
- Ошибки: 400 — id не число; 401 — нет действующего JWT; 403 — режим создал другой велосипедист; 404 — режима нет или он уже удалён.

#### `POST /api/motor_modes/:motor_mode_id/like` — лайк

- Обработчик: `LikeMotorMode`. Доступ: велосипедист. Тело: `{"like": 1}` ставит лайк текущего велосипедиста, `{"like": 0}` снимает. Повторный лайк строку в `motor_modes_likes` не добавляет.
- 200: `{"motor_mode_id": 15, "like_count": 1, "liked_by_current_rider": 1}`.
- Ошибки: 400 — `like` не 0 и не 1; 401 — нет действующего JWT; 404 — режим не опубликован.

### Велосипедисты (`riders`)

#### `POST /api/riders/register` — регистрация

- Обработчик: `RegisterRider`. Доступ: гость. Тело: `{"login": "alex_rider", "password": "velo2026"}`. Логин — 3–50 символов из строчных латинских букв, цифр и `_`; пароль — от 6 символов, не длиннее 72 байт.
- Пароль хранится bcrypt-хешем, наружу не отдаётся. 201: `{"id": 8, "login": "alex_rider", "is_bike_service_engineer": false}`.
- Ошибки: 400 — логин или пароль не подходят; 409 — логин занят.

#### `POST /api/riders/login` — аутентификация

- Обработчик: `LoginRider`. Доступ: гость. Тело: `{"login": "...", "password": "..."}`. Проверяет пароль по bcrypt-хешу и выдаёт JWT.
- 200:
  ```json
  {"access_token": "eyJhbGciOiJIUzI1NiIs…", "token_type": "Bearer", "expires_in": 3600,
   "rider": {"id": 1, "login": "ivan_rider", "is_bike_service_engineer": false}}
  ```
  `expires_in` — срок жизни токена в секундах. `is_bike_service_engineer` — признак инженера сервиса для интерфейса.
- Ошибки: 400 — не JSON; 401 — неверный логин или пароль.

#### `POST /api/riders/logout` — деавторизация

- Обработчик: `LogoutRider`. Доступ: велосипедист. Тела нет. JWT из заголовка записывается в чёрный список Redis до истечения срока действия, после этого любой запрос с ним получает 401.
- 200: `{"message": "Выход выполнен, JWT добавлен в чёрный список."}`. Ошибки: 401 — нет действующего JWT.

## Страницы (ЛР1–2)

| Метод и URL | Контроллер | Реализация |
|---|---|---|
| `GET /motor_modes/feed[/:motor_mode_id]` | `MotorModeFeed` | ORM. Лента: видео режима на весь экран. Без `id` — первый опубликованный, `?next=true` — следующий по `id`. Черновики и удалённые режимы отдают 404 с пустым экраном. `?next=true` — один запрос с переходом по кругу |
| `GET /motor_modes/draft` | `MotorModeDraft` | ORM. Черновик текущего велосипедиста: если его нет — форма с названием, фото и видео и кнопкой «Далее»; если есть — заполненные поля и кнопка «Опубликовать» |
| `GET /motor_modes` | `MotorModeGrid` | ORM. Плитка опубликованных режимов, фильтр `?maxConsumptionWhPerKm=` по расходу батареи, у каждой карточки кнопка «Удалить» |
| `POST /motor_modes/draft` | `CreateMotorModeDraft` | ORM `Create`. Создаёт черновик с названием; фото и видео на сервер не передаются, в `image_key` / `video_key` записываются файлы по умолчанию `default_motor_modes.jpg` / `.mp4` |
| `POST /motor_modes/draft/publish` | `PublishMotorModeDraft` | ORM `Updates`. Проверяет краткое описание и оба поля по теме, ставит статус `published` и `published_at` |
| `POST /motor_modes/:motor_mode_id/delete` | `DeleteMotorMode` | Без ORM: `UPDATE motor_modes SET status = 'deleted'` через `database/sql` |

## Медиа по умолчанию

`static/media/default_motor_modes.jpg` и `default_motor_modes.mp4` лежат на SSR-сервере рядом с иконками и загружаются в Minio скриптом `scripts/minio_setup.sh`.
Их ключи записываются черновику, созданному формой без файлов, и они же показываются, когда файл по ссылке в Minio недоступен:
изображения выводятся двухслойным `background-image` (Minio поверх заглушки), у `<video>` второй `<source>` указывает на заглушку.

## Структура

```
main.go                                    точка входа: .env, подключение к БД, Minio и Redis, маршруты, общее описание swagger
docs/                                      описание методов для swagger, генерируется командой swag init
cmd/migrate/main.go                        миграция таблиц через GORM AutoMigrate
db/seed.sql                                наполнение таблиц (14 режимов, 7 велосипедистов, 44 лайка)
internal/ds/                               модели riders, motor_modes, motor_modes_likes и правила проверки полей режима
internal/serializer/                       сериализаторы: JSON запросов и ответов веб-сервиса
internal/auth/jwt.go                       выпуск и проверка JWT
internal/auth/current_rider.go             функция-singleton велосипедиста для страниц ЛР1–2
internal/blacklist/                        чёрный список JWT в Redis
internal/api/                              веб-сервис: домены motor_modes и riders, middleware авторизации, маршруты /api, тесты
internal/storage/                          загрузка фото и видео в Minio
internal/dsn/dsn.go                        строка подключения из переменных окружения
internal/repository/                       выборки и изменения через GORM, удаление на страницах через SQL
internal/handler/                          страницы ЛР1–2: шесть контроллеров и таблица маршрутов
templates/                                 три страницы, панель вкладок, пустая страница 404, страница ошибки сервера
static/css/motor_modes.css                 стили, палитра Bosch eBike
static/media/                              фото и видео по умолчанию
scripts/db_setup.sh                        миграция и наполнение БД одной командой
scripts/minio_setup.sh                     бакет ebike-motor-media и загрузка медиа
```

## Данные

14 режимов работы мотора: 12 опубликованных, один черновик (`Limit`, создан велосипедистом 2, поэтому у текущего велосипедиста 1 черновика нет и страница добавления начинается с кнопки «Далее»), один удалённый (`Off`).
Режимы `Race` и `Sprint` создал велосипедист 1 (`ivan_rider`): после входа под ним у них `created_by_current_rider = 1`, и через API он может удалить только их (кнопка «Удалить» на странице плитки из ЛР2 по-прежнему удаляет любой опубликованный режим). У всех семи велосипедистов из `db/seed.sql` пароль `velo2026`.
Проценты поддержки — официальные значения Bosch eBike; расход батареи в Вт·ч/км согласован с запасом хода на аккумуляторе PowerTube 800 Вт·ч.
Границы слайдера на плитке вычисляются запросом `MIN`/`MAX` по опубликованным режимам с расходом больше нуля: минимум берётся как есть (4.6), максимум округляется вверх до целого (20).
