# Movie service

Сервис предоставляет HTTP и gRPC API для поиска фильмов. Данные синхронизируются с внешнего стенда
при старте и хранятся в PostgreSQL. Для поиска используется обратный индекс в Redis.

## Запуск
1. Запустите PostgreSQL и Redis:
    ```bash
    docker compose up -d

2. Примените миграции и запустите сервис:
    export DATABASE_URL="postgres://user:pass@localhost:5432/movies?sslmode=disable"
    export REDIS_ADDR="localhost:6480"
    go run ./cmd/server

## Переменные окружения
MOVIE_BASE_URL - базовый url стенда
HTTP_ADDR - адрес http-сервера 
DATABASE_URL - строка подключения к PostgreSQL
REDIS_ADDR - адрес Redis

## Поиск и структура индекса
При старте сервис выгружает все фильмы из PostgreSQL и строит обратный индекс  в Redis.
    ключи индекса: index:<слово>
    Значение: Redis Set с ID фильмов
    слова приводятся к нижнему регистру, разбиваются по символам(не буквы и не числа)
    Сброс кэша: При построении нового индекса старый кэш поисковых ответов полностью удаляется

## Кэширование
ответы метода /search кэшируются в Redis с префиксом cache:search:<запрос>. Время жизни кэша 3600 сек

## Примеры запросов
поиск по режиссёру:         curl "http://localhost:8080/search?q=Andrei"
поиск по нескольким словам: curl "http://localhost:8080/search?q=Andrei+Tarkovsky"
пустой запрос:              curl -i "http://localhost:8080/search?q= "
