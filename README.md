# ebike-battery-backend

Бэкенд заявочной системы по теме **«Оценка времени разряда аккумулятора электровелосипеда»**.
Курс «Разработка Интернет Приложений», ИУ5, 2026. Вариант 10 раздела «Транспорт, инфраструктура и строительство».

- **Услуга** — режим работы мотора (Eco, Tour, Sport, Turbo и другие режимы системы Bosch eBike).
- **Заявка** (с третьей лабораторной) — расчёт остатка заряда аккумулятора после поездки по маршруту с заданным перепадом высот.
- **Роли** — велосипедист (`rider`) и инженер сервиса (`bike_service_engineer`).

Ветки: `motor_mode_inMemory` — лабораторная 1 (коллекция в памяти), `motor_mode_postgres` — лабораторная 2 (PostgreSQL и GORM), `motor_modes_api` — лабораторная 3 (веб-сервис REST для SPA).

## Стек

Go 1.26, gin, GORM + PostgreSQL 16, Adminer, Minio в Docker (`minio-go/v7`), bcrypt. Веб-сервис отвечает JSON, страницы ЛР1–2 остаются на `html/template` без JavaScript.

## Запуск

```bash
docker compose up -d                 # Minio (9000/9001), PostgreSQL (5434), Adminer (8081)
./scripts/minio_setup.sh ../media    # создать публичный бакет и залить медиа
cp .env.example .env                 # параметры подключения к БД и Minio
./scripts/db_setup.sh --reset        # удалить таблицы, миграция GORM (cmd/migrate) и наполнение db/seed.sql
go run .                             # страницы http://localhost:8080/motor_modes, API http://localhost:8080/api
go test ./...                        # тесты веб-сервиса
```

Adminer: http://localhost:8081, система PostgreSQL, сервер `postgres`, пользователь `ebike`, пароль `ebike_password`, база `ebike_battery`.
Порт 5434 снаружи выбран, чтобы не пересекаться с локальным PostgreSQL на 5432.

Переменные окружения (`.env`): `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME` — PostgreSQL; `MINIO_PUBLIC_URL` — адрес бакета для ссылок на фото и видео; `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET` — загрузка файлов в Minio; без ключей Minio приложение не запускается.

Текущий велосипедист (создатель режимов) до авторизации в ЛР4 зафиксирован константой: функция-singleton `auth.CurrentRiderID()` в `internal/auth/current_rider.go` один раз через `sync.Once` запоминает `fixedCreatorRiderID = 1` и возвращает его во всех методах API и на страницах.

## База данных

Три таблицы, каскадного удаления нет (все внешние ключи `ON DELETE RESTRICT`):

| Таблица | Назначение | Столбцы |
|---|---|---|
| `riders` | велосипедисты и инженеры сервиса | `id`, `login` varchar(50) unique, `password_hash` varchar(100) — bcrypt, `is_bike_service_engineer` bool |
| `motor_modes` | режимы работы мотора (услуги) | `id`, `mode_name` varchar(50), `short_description` varchar(600) null, `status` varchar(20) (`draft` / `published` / `deleted`), `image_key` varchar(120) null, `video_key` varchar(120) null, `support_percent` int null, `consumption_wh_per_km` numeric(5,1) null, `created_at` timestamptz, `creator_rider_id` → `riders.id`, `published_at` timestamptz null |
| `motor_modes_likes` | м-м «велосипедист поставил лайк режиму» | `id`, `rider_id` → `riders.id`, `motor_mode_id` → `motor_modes.id`; unique(`rider_id`, `motor_mode_id`) |

По кнопке «Далее» заполняются только `mode_name`, `status`, `created_at` и `creator_rider_id`, поэтому остальные поля режима необязательные (null) и заполняются при публикации.
Частичный уникальный индекс `one_draft_per_rider` на `creator_rider_id where status = 'draft'` гарантирует не более одного черновика у велосипедиста.
Модели — `internal/ds`, миграция — `cmd/migrate/main.go` (`AutoMigrate`), данные — `db/seed.sql` (можно выполнить в Adminer).

## Веб-сервис

Справочник эндпоинтов. Код: маршруты — `internal/api/router.go`, обработчики — `internal/api/motor_modes.go` и `internal/api/riders.go`, JSON запросов и ответов — `internal/serializer/`.

### Общие правила

- Все адреса начинаются с `/api`. Запросы и ответы — JSON (`Content-Type: application/json`), кроме `POST /api/motor_modes` — `multipart/form-data`.
- Текущий велосипедист (создатель режимов) — всегда `id = 1`, его возвращает функция-singleton `auth.CurrentRiderID()` (`internal/auth/current_rider.go`). Клиент его не передаёт.
- Системные поля `id`, `status`, `creator_rider_id`, `created_at`, `published_at` клиент изменить не может: в сериализаторах запросов их нет, лишние поля игнорируются.
- Режимы в статусе `deleted` клиенту не отдаются. Переходы статусов: `draft` → `published` (PUT), `draft` / `published` → `deleted` (DELETE), назад в черновик нельзя (`MotorModeStatus.CanChangeTo`, `internal/ds/motor_modes_rules.go`).
- Ошибка клиента: `{"status": "fail", "message": "текст"}` с кодом 400, 401, 403, 404, 405, 409, 413, 415 или 502. Внутренняя ошибка: 500, `{"status": "error", ...}`.
- `image_url` / `video_url` — полный адрес файла в Minio или `null`, если файла нет (тогда фронтенд показывает медиа по умолчанию).

### Режимы работы мотора (`motor_modes`)

#### `GET /api/motor_modes` — список опубликованных режимов

- Обработчик: `ListMotorModes`.
- Query: `max_consumption_wh_per_km` — необязательный фильтр по расходу батареи, число больше 0 и меньше 1000. Без фильтра — все опубликованные.
- 200:
  ```json
  {"motor_modes": [{"id": 10, "mode_name": "Sprint", "image_url": "http://localhost:9000/ebike-motor-media/sprint_mode.jpg",
    "support_percent": 280, "consumption_wh_per_km": 12.1, "like_count": 3, "created_by_current_rider": 1}],
   "max_consumption_wh_per_km": 13, "consumption_min": 4.6, "consumption_max": 20}
  ```
  `created_by_current_rider` — 1, если режим создал текущий велосипедист, иначе 0. `consumption_min` / `consumption_max` — границы слайдера фильтра.
- Ошибки: 400 — фильтр не число или вне диапазона.

#### `GET /api/motor_modes/feed[/:motor_mode_id][?next=true]` — лента

- Обработчик: `GetMotorModeFeed`. Из БД всегда берётся одна строка.
- Без id — первый опубликованный режим; с id — этот режим; с id и `?next=true` — следующий опубликованный после id (после последнего снова первый; черновики и удалённые пропускаются).
- 200 — режим целиком (`MotorModeDetail`):
  ```json
  {"id": 15, "mode_name": "Walk Assist", "short_description": "…", "status": "published",
   "image_url": "…/motor_mode_15.jpg", "video_url": "…/motor_mode_15.mp4", "support_percent": 100,
   "consumption_wh_per_km": 3.4, "range_km": 235, "created_at": "…", "published_at": "…",
   "creator_rider_id": 1, "created_by_current_rider": 1, "like_count": 1, "liked_by_current_rider": 1}
  ```
  `range_km` — запас хода на аккумуляторе 800 Вт·ч.
- Ошибки: 400 — id не число; 404 — режима нет, он черновик или удалён.

#### `GET /api/motor_modes/draft` — черновик текущего велосипедиста

- Обработчик: `GetMotorModeDraft`. id не указывается, у велосипедиста не больше одного черновика.
- 200 — `MotorModeDetail` со статусом `draft`. Ошибки: 404 — черновика нет.

#### `POST /api/motor_modes` — добавить режим с фото и видео

- Обработчик: `CreateMotorMode`. Тело — `multipart/form-data`:
  - `mode_name` — текст, обязательно, до 50 символов;
  - `image` — файл JPEG / PNG / WebP до 10 МБ, необязательно;
  - `video` — файл MP4 до 50 МБ, необязательно.
- Создаёт черновик (`status = draft`, создатель — текущий велосипедист, `created_at` — сейчас). Файлы сохраняются в Minio под латинскими именами `motor_mode_<id>.<расширение>`, имена записываются в `image_key` / `video_key`. Всё в одной транзакции: если Minio недоступен, черновик не создаётся, загруженные файлы удаляются.
- 201 — `MotorModeDetail`, заголовок `Location: /api/motor_modes/draft`.
- Ошибки: 400 — нет названия, файл не того типа, `image` / `video` переданы текстом; 409 — черновик уже есть; 413 — файл больше лимита; 415 — тело не form-data; 502 — Minio недоступен.

#### `PUT /api/motor_modes/:motor_mode_id/publish` — опубликовать черновик

- Обработчик: `PublishMotorMode`. Тело:
  ```json
  {"short_description": "до 600 символов", "support_percent": 100, "consumption_wh_per_km": 3.4, "mode_name": "необязательно"}
  ```
  `support_percent` — целое 0–1000, `consumption_wh_per_km` — больше 0 и меньше 1000, округляется до 0.1.
- Меняет статус `draft` → `published`, `published_at` ставит сервер. 200 — `MotorModeDetail`.
- Ошибки: 400 — поле не заполнено или вне диапазона; 403 — режим создал другой велосипедист; 404 — режима нет или он удалён; 409 — режим уже опубликован.

#### `DELETE /api/motor_modes/:motor_mode_id` — удалить свой режим

- Обработчик: `DeleteMotorMode`. Мягкое удаление через ORM: статус `deleted`, строка остаётся в БД.
- 200: `{"id": 9, "status": "deleted"}`.
- Ошибки: 400 — id не число; 403 — режим создал другой велосипедист; 404 — режима нет или он уже удалён.

#### `POST /api/motor_modes/:motor_mode_id/like` — лайк

- Обработчик: `LikeMotorMode`. Тело: `{"like": 1}` ставит лайк текущего велосипедиста, `{"like": 0}` снимает. Повторный лайк строку в `motor_modes_likes` не добавляет.
- 200: `{"motor_mode_id": 15, "like_count": 1, "liked_by_current_rider": 1}`.
- Ошибки: 400 — `like` не 0 и не 1; 404 — режим не опубликован.

### Велосипедисты (`riders`)

#### `POST /api/riders/register` — регистрация

- Обработчик: `RegisterRider`. Тело: `{"login": "alex_rider", "password": "velo2026"}`. Логин — 3–50 символов из строчных латинских букв, цифр и `_`; пароль — от 6 символов, не длиннее 72 байт.
- Пароль хранится bcrypt-хешем, наружу не отдаётся. 201: `{"id": 8, "login": "alex_rider", "is_bike_service_engineer": false}`.
- Ошибки: 400 — логин или пароль не подходят; 409 — логин занят.

#### `POST /api/riders/login` — аутентификация

- Обработчик: `LoginRider`. Тело: `{"login": "...", "password": "..."}`. Проверяет пароль по bcrypt-хешу; токен не выдаётся (заглушка до ЛР4).
- 200: `{"rider": {"id": 8, "login": "alex_rider", "is_bike_service_engineer": false}, "message": "Логин и пароль верны."}`.
- Ошибки: 400 — не JSON; 401 — неверный логин или пароль.

#### `POST /api/riders/logout` — деавторизация

- Обработчик: `LogoutRider`. Тела нет (заглушка до ЛР4). 200: `{"status": "success", "message": "Выход выполнен."}`.

## Страницы (ЛР1–2)

| Метод и URL | Контроллер | Реализация |
|---|---|---|
| `GET /motor_modes/feed[/:motor_mode_id]` | `MotorModeFeed` | ORM. Лента: видео режима на весь экран. Без `id` — первый опубликованный, `?next=true` — следующий по `id`. Черновики и удалённые режимы отдают 404 с пустым экраном. `?next=true` — один запрос с переходом по кругу |
| `GET /motor_modes/draft` | `MotorModeDraft` | ORM. Черновик текущего велосипедиста: если его нет — форма с названием, фото и видео и кнопкой «Далее»; если есть — заполненные поля и кнопка «Опубликовать» |
| `GET /motor_modes` | `MotorModeGrid` | ORM. Плитка опубликованных режимов, фильтр `?maxConsumptionWhPerKm=` по расходу батареи, у каждой карточки кнопка «Удалить» |
| `POST /motor_modes/draft` | `CreateMotorModeDraft` | ORM `Create`. Создаёт черновик с названием; фото и видео на сервер не передаются, у нового режима показываются медиа по умолчанию |
| `POST /motor_modes/draft/publish` | `PublishMotorModeDraft` | ORM `Updates`. Проверяет краткое описание и оба поля по теме, ставит статус `published` и `published_at` |
| `POST /motor_modes/:motor_mode_id/delete` | `DeleteMotorMode` | Без ORM: `UPDATE motor_modes SET status = 'deleted'` через `database/sql` |

## Медиа по умолчанию

`static/media/default_motor_modes.jpg` и `default_motor_modes.mp4` лежат на SSR-сервере рядом с иконками.
Они показываются, когда `image_key` / `video_key` пустые или null (подставляет контроллер) и когда файл по ссылке в Minio недоступен:
изображения выводятся двухслойным `background-image` (Minio поверх заглушки), у `<video>` второй `<source>` указывает на заглушку.

## Структура

```
main.go                                    точка входа: .env, подключение к БД, маршруты
cmd/migrate/main.go                        миграция таблиц через GORM AutoMigrate
db/seed.sql                                наполнение таблиц (14 режимов, 7 велосипедистов, 44 лайка)
internal/ds/                               модели riders, motor_modes, motor_modes_likes и правила проверки полей режима
internal/serializer/                       сериализаторы: JSON запросов и ответов веб-сервиса
internal/auth/current_rider.go             функция-singleton текущего велосипедиста
internal/api/                              веб-сервис: домены motor_modes и riders, маршруты /api, тесты
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
Режимы `Race` и `Sprint` создал текущий велосипедист 1, у них `created_by_current_rider = 1`, и через API он может удалить только их (кнопка «Удалить» на странице плитки из ЛР2 по-прежнему удаляет любой опубликованный режим). У всех семи велосипедистов из `db/seed.sql` пароль `velo2026`.
Проценты поддержки — официальные значения Bosch eBike; расход батареи в Вт·ч/км согласован с запасом хода на аккумуляторе PowerTube 800 Вт·ч.
Границы слайдера на плитке вычисляются запросом `MIN`/`MAX` по опубликованным режимам с расходом больше нуля: минимум берётся как есть (4.6), максимум округляется вверх до целого (20).
