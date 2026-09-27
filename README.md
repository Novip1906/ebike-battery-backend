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

Все методы начинаются с `/api`, принимают и отдают JSON (добавление режима — `multipart/form-data`). Ошибка клиента — `{"status": "fail", "message": "..."}` с кодом 400 (неверные данные), 401 (неверный пароль), 403 (чужой режим), 404 (нет записи), 405 (не тот HTTP-метод), 409 (конфликт статуса или логина), 413 (слишком большой файл), 415 (не form-data) или 502 (Minio недоступен); внутренняя ошибка — 500 с `{"status": "error", ...}`.
Режимы в статусе `deleted` клиенту не передаются. Системные поля (`id`, `status`, `creator_rider_id`, `created_at`, `published_at`) с клиента не принимаются: их нет в сериализаторах запросов, сервер вычисляет их сам.

**Домен режимов работы мотора** (`internal/api/motor_modes.go`)

| Метод и URL | Тело запроса | Ответ |
|---|---|---|
| `GET /api/motor_modes?max_consumption_wh_per_km=13` | — | 200: опубликованные режимы с расходом не больше фильтра (без фильтра или с пустым значением — все), у каждого `id`, `mode_name`, `image_url`, `support_percent`, `consumption_wh_per_km`, `like_count`, `created_by_current_rider` 0/1; применённый фильтр `max_consumption_wh_per_km` и границы слайдера `consumption_min`, `consumption_max`. 400 — фильтр не число от 0.1 до 999.9 |
| `GET /api/motor_modes/feed` | — | 200: первый опубликованный режим. Одна строка из БД. 404 — опубликованных режимов нет |
| `GET /api/motor_modes/feed/:motor_mode_id[?next=true]` | — | 200: опубликованный режим по id; с `?next=true` — следующий опубликованный после этого id, после последнего снова первый. 404 — по id черновик, удалённый режим или такого нет, 400 — id не число |
| `GET /api/motor_modes/draft` | — | 200: черновик текущего велосипедиста, id не указывается. 404 — черновика нет |
| `POST /api/motor_modes` | form-data: `mode_name`, файлы `image` (JPEG/PNG/WebP до 10 МБ) и `video` (MP4 до 50 МБ) | 201: новый черновик. Тип файла определяется по содержимому, файлы сохраняются в Minio под именами `motor_mode_<id>` с расширением по типу (`.jpg`, `.png`, `.webp`, `.mp4`), имена записываются в `image_key`, `video_key`. Файлы можно не передавать — тогда ключи пустые и показываются медиа по умолчанию. 400 — нет названия, файл не того типа, `image`/`video` переданы текстом или форма не разбирается, 409 — у велосипедиста уже есть черновик, 413 — файл больше лимита, 415 — не form-data, 502 — Minio недоступен (черновик не создаётся, загруженные файлы удаляются) |
| `PUT /api/motor_modes/:motor_mode_id/publish` | `{"short_description", "support_percent", "consumption_wh_per_km", "mode_name"?}` | 200: статус `draft` → `published`, `published_at` = сейчас. 400 — поля не заполнены или вне диапазона, 403 — режим создал другой велосипедист, 404 — режима нет или он удалён, 409 — уже опубликован |
| `DELETE /api/motor_modes/:motor_mode_id` | — | 200 `{"id", "status": "deleted"}`: мягкое удаление своего черновика или опубликованного режима через ORM. 403 — чужой режим, 404 — режима нет или он уже удалён |
| `POST /api/motor_modes/:motor_mode_id/like` | `{"like": 1}` или `{"like": 0}` | 200 `{"motor_mode_id", "like_count", "liked_by_current_rider"}`: 1 ставит лайк текущего велосипедиста (повторный лайк строку не добавляет), 0 отменяет. 400 — `like` не 0 и не 1, 404 — режим не опубликован |

Лента и черновик возвращают режим целиком: поля таблицы (вместо `image_key` / `video_key` — готовые `image_url`, `video_url`, null при пустом ключе, тогда фронтенд показывает медиа по умолчанию), `range_km` (запас хода на аккумуляторе 800 Вт·ч), `like_count`, `liked_by_current_rider`, `created_by_current_rider`.
Переходы статусов заданы в модели (`MotorModeStatus.CanChangeTo` в `internal/ds/motor_modes_rules.go`): `draft` → `published` (PUT), `draft` / `published` → `deleted` (DELETE); вернуть режим в черновик нельзя.

**Домен велосипедистов** (`internal/api/riders.go`)

| Метод и URL | Тело запроса | Ответ |
|---|---|---|
| `POST /api/riders/register` | `{"login", "password"}` | 201 `{"id", "login", "is_bike_service_engineer": false}`, пароль хранится bcrypt-хешем, хеш наружу не отдаётся. 400 — логин не из 3–50 строчных латинских букв, цифр и `_` или пароль короче 6 символов либо длиннее 72 байт (предел bcrypt), 409 — логин занят |
| `POST /api/riders/login` | `{"login", "password"}` | заглушка до ЛР4: 200 с данными велосипедиста, если пароль верный, иначе 401. Токен не выдаётся |
| `POST /api/riders/logout` | — | заглушка до ЛР4: 200 |

Коллекция из 11 запросов для Postman лежит вне репозитория, вместе с материалами защиты.

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
