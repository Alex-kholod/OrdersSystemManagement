# Система управления заказами интернет-магазина

Backend сервис для управления заказами, товарами и пользователями.

**Студент:** Холодков А.Д.  
**Группа:** ЭФМО-01-25

## Стек технологий

| Компонент | Технология |
|-----------|-----------|
| Язык | Go 1.22 |
| HTTP фреймворк | Gin |
| База данных | PostgreSQL 15 |
| ORM | GORM |
| Авторизация | JWT (HS256) |
| Документация | Swagger / OpenAPI |
| Контейнеризация | Docker + Docker Compose |

## Запуск

```bash
cp .env.example .env
docker-compose up -d
```

Сервис будет доступен на `http://localhost:8080`

## Документация

Swagger UI — [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)

## Структура проекта

```
order-service/
├── cmd/server/        # Точка входа
├── internal/
│   ├── config/        # Конфигурация
│   ├── domain/        # Модели и интерфейсы
│   ├── handler/       # HTTP обработчики
│   ├── middleware/    # JWT, проверка роли
│   ├── repository/    # Работа с БД
│   └── service/       # Бизнес-логика
├── migrations/        # SQL схема
├── Dockerfile
└── docker-compose.yml
```
