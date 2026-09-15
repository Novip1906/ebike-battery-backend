# ebike-battery-backend

Бэкенд заявочной системы по теме **«Оценка времени разряда аккумулятора электровелосипеда»**.
Курс «Разработка Интернет Приложений», ИУ5, 2026. Вариант 10 раздела «Транспорт, инфраструктура и строительство».

- **Услуга** — режим работы мотора (Eco, Tour, Sport, Turbo и другие режимы системы Bosch eBike).
- **Заявка** (с третьей лабораторной) — расчёт остатка заряда аккумулятора после поездки по маршруту с заданным перепадом высот.
- **Роли** — велосипедист (`rider`) и инженер сервиса (`bike_service_engineer`).

Ветки: `motor_mode_inMemory` — лабораторная 1 (коллекция в памяти), `motor_mode_postgres` — лабораторная 2 (PostgreSQL и GORM).

## Стек

Go 1.26, gin, `html/template`, GORM + PostgreSQL 16, Adminer, Minio в Docker. Без JavaScript.

## Запуск

```bash
docker compose up -d                 # Minio (9000/9001), PostgreSQL (5434), Adminer (8081)
./scripts/minio_setup.sh ../media    # создать публичный бакет и залить медиа
cp .env.example .env                 # параметры подключения к БД и Minio
./scripts/db_setup.sh                # миграция GORM (cmd/migrate) и наполнение db/seed.sql
go run .                             # приложение на http://localhost:8080
```

Adminer: http://localhost:8081, система PostgreSQL, сервер `postgres`, пользователь `ebike`, пароль `ebike_password`, база `ebike_battery`.
Порт 5434 снаружи выбран, чтобы не пересекаться с локальным PostgreSQL на 5432.

Переменные окружения (`.env`): `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`, `MINIO_PUBLIC_URL`, `CURRENT_RIDER_ID` (велосипедист, от имени которого создаётся черновик, пока нет авторизации).

## База данных

Три таблицы, каскадного удаления нет (все внешние ключи `ON DELETE RESTRICT`):

| Таблица | Назначение | Столбцы |
|---|---|---|
| `riders` | велосипедисты и инженеры сервиса | `id`, `login` varchar(50) unique, `password_hash` varchar(100), `is_bike_service_engineer` bool |
| `motor_modes` | режимы работы мотора (услуги) | `id`, `mode_name` varchar(50), `short_description` varchar(600), `status` varchar(20) (`draft` / `published` / `deleted`), `image_key` varchar(120), `video_key` varchar(120), `support_percent` int, `consumption_wh_per_km` numeric(5,1), `created_at` timestamptz, `creator_rider_id` → `riders.id`, `published_at` timestamptz null |
| `motor_mode_likes` | м-м «велосипедист поставил лайк режиму» | `id`, `rider_id` → `riders.id`, `motor_mode_id` → `motor_modes.id`, `liked_at` timestamptz; unique(`rider_id`, `motor_mode_id`) |

Частичный уникальный индекс `one_draft_per_rider` на `creator_rider_id where status = 'draft'` гарантирует не более одного черновика у велосипедиста.
Модели — `internal/ds`, миграция — `cmd/migrate/main.go` (`AutoMigrate`), данные — `db/seed.sql` (можно выполнить в Adminer).

## Маршруты

| Метод и URL | Контроллер | Реализация |
|---|---|---|
| `GET /motor-modes/feed[/:motor_mode_id]` | `MotorModeFeed` | ORM. Лента: видео режима на весь экран. Без `id` — первый опубликованный, `?next=true` — следующий по `id`. Черновики и удалённые режимы отдают 404 |
| `GET /motor-modes/draft` | `MotorModeDraft` | ORM. Черновик текущего велосипедиста: если его нет — форма с названием, фото и видео и кнопкой «Далее»; если есть — заполненные поля и кнопка «Опубликовать» |
| `GET /motor-modes` | `MotorModeGrid` | ORM. Плитка опубликованных режимов, фильтр `?maxConsumptionWhPerKm=` по расходу батареи, у каждой карточки кнопка «Удалить» |
| `POST /motor-modes/draft` | `CreateMotorModeDraft` | ORM `Create`. Создаёт черновик с названием; фото и видео на сервер не передаются, у нового режима показываются медиа по умолчанию |
| `POST /motor-modes/draft/publish` | `PublishMotorModeDraft` | ORM `Save`. Проверяет краткое описание и оба поля по теме, ставит статус `published` и `published_at` |
| `POST /motor-modes/:motor_mode_id/delete` | `DeleteMotorMode` | Без ORM: `UPDATE motor_modes SET status = 'deleted'` через `database/sql` |

## Медиа по умолчанию

`static/media/default_motor_mode.jpg` и `default_motor_mode.mp4` лежат на SSR-сервере рядом с иконками.
Они показываются, когда `image_key` / `video_key` пустые (подставляет контроллер) и когда файл по ссылке в Minio недоступен:
изображения выводятся двухслойным `background-image` (Minio поверх заглушки), у `<video>` второй `<source>` указывает на заглушку.

## Структура

```
main.go                                    точка входа: .env, подключение к БД, маршруты
cmd/migrate/main.go                        миграция таблиц через GORM AutoMigrate
db/seed.sql                                наполнение таблиц (14 режимов, 7 велосипедистов, 44 лайка)
internal/ds/                               модели riders, motor_modes, motor_mode_likes
internal/dsn/dsn.go                        строка подключения из переменных окружения
internal/repository/                       выборки и изменения через GORM, удаление через SQL
internal/handler/                          шесть контроллеров и таблица маршрутов
templates/                                 три страницы, панель вкладок, страница 404
static/css/motor_mode.css                  стили, палитра Bosch eBike
static/media/                              фото и видео по умолчанию
scripts/db_setup.sh                        миграция и наполнение БД одной командой
scripts/minio_setup.sh                     бакет ebike-motor-media и загрузка медиа
```

## Данные

14 режимов работы мотора: 12 опубликованных, один черновик (`Limit`, создан велосипедистом 2, поэтому у текущего велосипедиста 1 черновика нет и страница добавления начинается с кнопки «Далее»), один удалённый (`Off`).
Проценты поддержки — официальные значения Bosch eBike; расход батареи в Вт·ч/км согласован с запасом хода на аккумуляторе PowerTube 800 Вт·ч.
Границы слайдера на плитке вычисляются запросом `MIN`/`MAX` по опубликованным режимам с расходом больше нуля: минимум берётся как есть (4.6), максимум округляется вверх до целого (20).
