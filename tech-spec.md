

# Техническое задание

## Проект: `orm` — типобезопасный ORM/Model layer для Go

## 1. Цель проекта

Разработать пакет `orm` для Go, который предоставляет **высокоуровневый typed ORM/model layer** поверх `dbx` и решает задачи:

* маппинг Go-структур в таблицы БД
* типобезопасный CRUD
* декларативную metadata-модель сущностей
* удобную работу с primary keys, filters, sorting, pagination
* relations/preload
* hooks lifecycle
* единый error model
* единый metadata layer, совместимый с `arc`

## 1.1. Основная проблема

На текущем этапе `dbx` покрывает низкоуровневую SQL-работу и generic scan/query use cases, но не решает полностью задачи ORM-уровня:

* отсутствует единая registry/model metadata система
* нет полноценной abstraction модели сущностей
* нет стандартизованного CRUD по типам
* нет relation graph / preload модели
* нет нормализованной high-level API для repository-like использования
* нет общей metadata-основы, которую можно переиспользовать в `arc`

## 1.2. Целевое состояние

Нужен `orm`, который:

* использует `dbx` как низкоуровневый транспорт к БД
* работает с Go-типами как с моделями
* строит metadata модели один раз
* даёт удобный DX через generics
* остаётся прозрачным и предсказуемым
* не превращается в “магический black box”
* может использоваться совместно с `arc` через общую type metadata систему

---

# 2. Область применения

`orm` предназначен для:

* backend-сервисов
* REST API backend-ов
* внутренних бизнес-сервисов
* CRUD-heavy систем
* доменных сервисов со строгими типами

`orm` не является целью для:

* скрытой генерации SQL без возможности контроля
* сложной BI/OLAP аналитики
* полноценной migration framework как части первой версии
* замены ручному SQL в сложных высокооптимизированных запросах

---

# 3. Архитектурная роль в экосистеме

## 3.1. Место пакета

Архитектура пакетов:

* `dbx` — SQL builder / query / scan / tx / low-level execution
* `orm` — model/entity layer поверх `dbx`
* `arc` — HTTP/API layer
* shared metadata layer — общий reflection/meta cache для `orm` и `arc`

## 3.2. Принцип

`orm` должен быть:

* **выше `dbx`**
* **ниже бизнес-логики**
* **независимым от `arc`**
* **совместимым с общей metadata-моделью типов**

## 3.3. Ключевой архитектурный принцип

> Один Go-тип должен разбираться системой metadata один раз, а затем его описание должно использоваться и в `orm`, и в `arc`, и в других инструментах экосистемы.

`orm` не должен иметь отдельный, несовместимый reflection engine.

---

# 4. Общие требования

## 4.1. Язык и платформа

* Язык: Go
* Основа: `database/sql`
* Низкоуровневая зависимость: `dbx`
* Внешние зависимости — минимально необходимые

## 4.2. Нефункциональные требования

* предсказуемость поведения
* минимизация runtime reflection
* reflection допустим на startup / first-use / metadata registration
* высокая производительность scan/query/CRUD операций
* thread-safe metadata cache
* прозрачные ошибки
* расширяемость через интерфейсы и hooks

## 4.3. DX требования

API пакета должен быть:

* очевидным
* компактным
* согласованным по стилю
* пригодным для generics
* не требующим писать много шаблонного кода

---

# 5. Модель данных

## 5.1. Сущность / модель

`orm` должен работать с Go-структурами как с моделями.

Пример:

```go
type User struct {
	ID        int64     `db:"id,pk"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	Status    string    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
```

## 5.2. Требования к model metadata

Для каждой модели система должна уметь определять:

* имя Go-типа
* table name
* список полей
* соответствие Go field -> DB column
* primary key field(s)
* auto-generated fields
* nullable / optional semantics
* embedded fields
* ignored fields
* read-only / write-only fields
* defaultable fields
* relation metadata
* soft delete field
* created/updated timestamp fields

## 5.3. Источник metadata

Metadata должна определяться из:

* struct tags
* naming strategy
* опциональных registration options
* интерфейсных override hooks

---

# 6. Shared Type Metadata Layer

## 6.1. Требование

`orm` должен использовать общий metadata layer, который совместим с `arc`.

Этот слой должен:

* разбирать struct once
* кэшировать информацию
* предоставлять унифицированное описание полей и типов

## 6.2. Минимальная metadata информация

Для каждого поля должны быть доступны:

* GoName
* DBName
* JSONName
* Type
* IsPK
* IsNullable
* IsIgnored
* IsReadOnly
* IsWriteOnly
* HasDefault
* Embedded path
* Required/optional semantics

## 6.3. Причина

Это нужно для:

* отсутствия дублирования reflection logic
* единых правил nullable/zero semantics
* совместимости `orm` и `arc`
* дальнейшей генерации схем/документации/DTO tooling

---

# 7. Table mapping

## 7.1. Определение имени таблицы

Имя таблицы должно поддерживать несколько способов задания:

### По умолчанию

Через naming strategy, например:

* `User` -> `users`
* `UserProfile` -> `user_profiles`

### Через интерфейс

Например:

```go
type TableNamer interface {
	TableName() string
}
```

### Через registration override

Например:

```go
orm.Register[User](orm.ModelConfig{
	Table: "app_users",
})
```

## 7.2. Naming strategy

Нужно поддержать кастомизацию стратегии именования:

* table names
* column names
* relation foreign keys
* join table names

---

# 8. CRUD API

## 8.1. Общая цель

`orm` должен давать компактный typed CRUD API.

## 8.2. Основные операции

Обязательно поддержать:

* Insert
* Update
* Delete
* DeleteByPK
* FindByPK / GetByPK
* FindOne
* FindAll / List
* Exists
* Count
* Upsert — желательно, но можно phase 2

## 8.3. Примеры желаемого API

```go
err := orm.Insert(ctx, db, &user)
```

```go
err := orm.Update(ctx, db, &user)
```

```go
user, err := orm.ByPK[User](ctx, db, id)
```

```go
users, err := orm.Query[User](db).
	WhereEq("status", "active").
	OrderBy("created_at DESC").
	Limit(50).
	All(ctx)
```

## 8.4. Поведение Insert

Insert должен:

* определить таблицу
* определить insertable columns
* исключить ignored/read-only/generated поля
* обработать PK/default/generated semantics
* при необходимости вернуть auto-generated values
* поддерживать insert одной записи
* batch insert — желательно

## 8.5. Поведение Update

Update должен:

* обновлять запись по primary key
* поддерживать configurable partial update strategy
* уметь исключать zero-value поля в patch-like режиме
* поддерживать явный список полей для update
* обновлять auto timestamp fields при включенной опции

## 8.6. Поведение Delete

Delete должен поддерживать:

* hard delete
* soft delete, если модель помечена соответствующим полем

---

# 9. Query API

## 9.1. Требование

Нужен typed query builder уровня ORM, который строится поверх `dbx`, но ориентирован на модели.

## 9.2. Возможности query API

Поддержать:

* WhereEq
* WhereNotEq
* WhereIn
* WhereNull
* WhereNotNull
* WhereLike
* WhereGT / GTE / LT / LTE
* OrderBy
* Limit
* Offset
* Page/Pagination helper
* Select columns
* Exclude columns
* Joins
* Preload
* Count
* Exists

## 9.3. Пример

```go
users, err := orm.Query[User](db).
	WhereEq("status", "active").
	WhereIn("role", []string{"admin", "manager"}).
	OrderBy("created_at DESC").
	Limit(20).
	All(ctx)
```

## 9.4. Typed/unsafe boundary

Нужно явно разделить:

* безопасный API по metadata-aware полям
* low-level escape hatch для ручного SQL/expr

Например:

```go
WhereExpr("LOWER(email) = LOWER(?)", email)
```

Но этот путь должен быть явно обозначен как low-level.

---

# 10. Primary key support

## 10.1. Требования

ORM должен поддерживать:

* single-column PK
* composite PK — желательно на phase 1, но можно ограниченно
* auto-increment PK
* UUID PK
* manual PK

## 10.2. Метаданные PK

Нужно уметь определить:

* какие поля составляют PK
* генерируется ли PK БД
* нужно ли возвращать PK после insert

## 10.3. CRUD по PK

Обязательно поддержать:

* `ByPK[T]`
* `DeleteByPK[T]`
* `ExistsByPK[T]`

---

# 11. Field behavior and persistence rules

## 11.1. Типы полей

Нужно поддержать поля:

* string
* bool
* integers
* floats
* time.Time
* pointers
* sql.Null* типы
* пользовательские Scanner/Valuer типы
* UUID
* slices/JSON fields — опционально через codec
* embedded structs

## 11.2. Ignore semantics

Поддержать исключение полей из persistence:

```go
Field string `db:"-"`
```

## 11.3. Read-only / write-only

Поддержать поля, которые:

* читаются из БД, но не пишутся
* пишутся, но не читаются — редко, но оставить возможность

## 11.4. Default/generated fields

Поддержать поля, значения которых:

* генерируются БД
* имеют default на стороне БД
* должны быть исключены из insert по умолчанию

---

# 12. Relations

## 12.1. Цель

`orm` должен поддерживать relation metadata и preload механизм.

## 12.2. Типы связей

Желательно поддержать:

* belongs-to
* has-one
* has-many
* many-to-many — phase 2 допустимо

## 12.3. Relation metadata

Для relation должны определяться:

* relation name
* kind
* local key
* foreign key
* target model
* join table — для many-to-many
* preload strategy

## 12.4. Preload

Поддержать API вида:

```go
users, err := orm.Query[User](db).
	Preload("Profile").
	Preload("Roles").
	All(ctx)
```

## 12.5. Ограничения первой версии

На MVP допускается:

* preload без сложной оптимизации графа
* без ленивой загрузки через proxy
* без магического auto-load

Явный preload предпочтителен.

---

# 13. Lifecycle hooks

## 13.1. Требование

Нужны lifecycle hooks на уровне модели и/или ORM engine.

## 13.2. Поддерживаемые хуки

Минимально:

* BeforeInsert
* AfterInsert
* BeforeUpdate
* AfterUpdate
* BeforeDelete
* AfterDelete
* AfterFind — желательно

## 13.3. Возможный интерфейс

```go
type BeforeInsertHook interface {
	BeforeInsert(ctx context.Context) error
}
```

## 13.4. Global hooks

Нужно предусмотреть глобальные hooks/interceptors для:

* auditing
* logging
* tracing
* tenant filters
* security constraints

---

# 14. Timestamps and soft delete

## 14.1. Auto timestamps

Поддержать автоматическое поведение для полей:

* created_at
* updated_at

Через tag/config/convention.

## 14.2. Soft delete

Поддержать soft delete через поле вида:

* `deleted_at`
* `is_deleted` — опционально

Поведение:

* обычные query по умолчанию исключают soft-deleted записи
* должна быть возможность явно включать их
* delete для soft-delete модели выполняет update, а не delete

Пример API:

```go
orm.Query[User](db).WithDeleted()
```

---

# 15. Transactions

## 15.1. Требование

`orm` должен корректно работать как с `*sql.DB`, так и с `*sql.Tx` через `dbx` abstraction.

## 15.2. Поддержка

Все CRUD/query операции должны принимать совместимый executor/interface.

## 15.3. Helper API

Желательно:

```go
err := orm.WithTx(ctx, db, func(tx orm.Tx) error {
	...
	return nil
})
```

или использование `dbx`-совместимого транзакционного API.

---

# 16. Error model

## 16.1. Требование

Нужна единая и предсказуемая модель ошибок.

## 16.2. Базовые ошибки

Нужно стандартизовать ошибки:

* ErrNotFound
* ErrMultipleRows
* ErrConflict
* ErrInvalidModel
* ErrMissingPrimaryKey
* ErrNoRowsAffected
* ErrRelationNotFound
* ErrUnsupportedType
* ErrInvalidField
* ErrInvalidQuery
* ErrSoftDeleted — опционально

## 16.3. Поведение

Ошибки должны:

* быть пригодны для `errors.Is`
* содержать machine-readable тип
* при необходимости содержать metadata/context
* легко маппиться в `arc` error model

Пример:

* `orm.ErrNotFound` -> 404
* `orm.ErrConflict` -> 409

---

# 17. Model registry

## 17.1. Требование

Нужен registry моделей, который является источником истины для ORM metadata.

## 17.2. Registry должен хранить

* model type
* table name
* field map
* PK info
* relation info
* hooks info
* timestamps config
* soft delete config
* naming config overrides

## 17.3. Регистрация моделей

Нужно поддержать:

### Lazy registration

При первом использовании типа

### Explicit registration

Например:

```go
orm.Register[User](orm.ModelConfig{
	Table: "users",
})
```

## 17.4. Startup validation

Нужно иметь возможность заранее провалидировать registry:

* конфликты колонок
* отсутствие PK
* некорректные relations
* unsupported types

---

# 18. Partial updates / patch semantics

## 18.1. Проблема

Нужно различать:

* zero value
* explicit zero
* absent field

## 18.2. Требование

ORM должен поддержать стратегии update:

* full update
* patch update by explicit field list
* patch update by optional wrapper types — желательно
* map/set style update — допустимо как low-level helper

## 18.3. Пример API

```go
err := orm.UpdateFields(ctx, db, &user, "name", "status")
```

или

```go
err := orm.Query[User](db).
	WhereEq("id", id).
	Set("name", name).
	Set("status", status).
	Update(ctx)
```

---

# 19. Bulk operations

## 19.1. Phase 1

Желательно поддержать:

* batch insert
* bulk delete by filter
* bulk update by filter

## 19.2. Ограничения

Bulk API должен быть явным и не маскироваться под обычный model update.

---

# 20. Repository support

## 20.1. Требование

ORM не должен требовать repository pattern, но должен позволять его удобно строить.

## 20.2. Поддержка

Нужны primitives, достаточные для создания typed repositories:

```go
type UserRepo struct {
	db orm.DB
}

func (r *UserRepo) ByID(ctx context.Context, id int64) (*User, error) {
	return orm.ByPK[User](ctx, r.db, id)
}
```

## 20.3. Не делать обязательным

Не нужно навязывать ActiveRecord-style методы на самой модели.

---

# 21. Extensibility

## 21.1. Custom types

Нужно поддержать пользовательские типы через:

* `sql.Scanner`
* `driver.Valuer`
* custom codecs
* custom field handlers

## 21.2. Custom field metadata

Нужно предусмотреть extension points для кастомных тегов и field behaviors.

## 21.3. Interceptors/plugins

Нужно предусмотреть расширение для:

* audit
* tracing
* multitenancy
* row-level filters
* sharding hooks — позже

---

# 22. Performance requirements

## 22.1. Общие требования

* минимум reflection на hot path
* metadata кэшируется
* column-to-field mapping подготавливается заранее
* scan path должен быть оптимизирован для частых операций

## 22.2. Benchmarks

Нужно иметь benchmark-набор для:

* single row by PK
* list query
* insert one row
* update one row
* preload one relation
* batch insert

Сравнение желательно вести минимум с:

* ручным `database/sql`
* текущим `dbx`
* популярным ORM по выбору — опционально

---

# 23. Observability

## 23.1. Logging

Нужно предусмотреть hooks для логирования:

* query text
* duration
* rows affected
* model name
* error

С поддержкой redact policy для чувствительных значений.

## 23.2. Metrics

Нужно предусмотреть hooks для:

* query count
* latency
* errors
* rows scanned
* tx count

## 23.3. Tracing

Интеграция с tracing должна быть возможна через interceptor/hook layer.

---

# 24. Совместимость с `arc`

## 24.1. Критичное требование

`orm` должен быть спроектирован так, чтобы типы могли удобно использоваться совместно с `arc`.

## 24.2. Для этого нужно

* использовать shared type metadata layer
* не вводить ORM-only несовместимую tag-систему
* обеспечивать единые nullable/optional semantics
* использовать ошибки, которые легко маппятся в HTTP layer

## 24.3. Чего не должно быть

`orm` не должен:

* зависеть от `arc`
* содержать HTTP semantics
* содержать OpenAPI-specific behavior
* требовать API-specific теги

---

# 25. Ограничения первой версии (MVP)

## 25.1. Обязательно в MVP

* model metadata
* table mapping
* single PK support
* typed CRUD
* typed query API
* error model
* timestamps
* soft delete
* hooks
* shared metadata integration
* startup validation
* tests
* benchmarks

## 25.2. Можно отложить на phase 2

* composite PK full support
* many-to-many
* complex relation graph optimization
* schema migration subsystem
* code generation
* advanced patch types
* sharding
* tenant-aware automatic routing
* advanced JSON/document fields

---

# 26. Тестирование

## 26.1. Нужно покрыть тестами

* metadata parsing
* table/column naming
* CRUD operations
* PK behavior
* timestamps
* soft delete
* hooks
* relation preload
* error shape stability
* custom types
* transactions

## 26.2. Виды тестов

* unit tests
* integration tests against real DB
* snapshot-like tests для metadata
* benchmarks

## 26.3. Базы данных

Минимально желательно тестировать на:

* PostgreSQL
* SQLite — если планируется поддержка

---

# 27. Документация

Нужно подготовить:

* README
* quick start
* model definition guide
* query guide
* relations guide
* hooks guide
* transactions guide
* integration guide with `dbx`
* integration guide with `arc`

Примеры:

* simple CRUD
* repository pattern
* transactions
* preload relations
* soft delete
* custom type

---

# 28. Этапы реализации

## Этап 1. Metadata core

* model metadata
* registry
* naming strategy
* field parsing
* PK detection

## Этап 2. CRUD

* insert
* update
* delete
* by pk
* count/exists

## Этап 3. Query API

* filters
* sort
* limit/offset
* all/one/first
* field selection

## Этап 4. Hooks / timestamps / soft delete

* lifecycle hooks
* auto timestamps
* soft delete behavior

## Этап 5. Relations

* relation metadata
* explicit preload
* relation validation

## Этап 6. Stabilization

* integration tests
* benchmarks
* docs
* examples

---

# 29. Критерии приёмки

Система считается принятой, если:

1. можно описать Go-модель один раз и использовать её в ORM без дублирующего описания колонок/PK вручную в каждом запросе

2. CRUD и query API покрывают большинство типовых сценариев backend-разработки

3. metadata модели кэшируются и не разбираются заново на каждом запросе

4. ошибки ORM стандартизованы и пригодны для маппинга во внешний слой

5. `orm` архитектурно совместим с shared metadata layer и может использоваться рядом с `arc`

6. производительность не имеет грубых деградаций относительно `dbx` для типовых сценариев

7. есть тесты, benchmarks и документация

---

# 30. Краткая формулировка задачи для команды

Разработать пакет `orm` для Go как typed ORM/model layer поверх `dbx`, обеспечивающий metadata-driven mapping структур в таблицы БД, типобезопасный CRUD и query API, relations/preload, hooks, timestamps, soft delete, единый error model и совместимость с общей type metadata системой, используемой также в `arc`.

---

# 31. Рекомендуемый стиль API

Я бы целился в такой DX:

```go
type User struct {
	ID        int64      `db:"id,pk"`
	Email     string     `db:"email"`
	Name      string     `db:"name"`
	Status    string     `db:"status"`
	DeletedAt *time.Time `db:"deleted_at"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
}

user, err := orm.ByPK[User](ctx, db, 42)

users, err := orm.Query[User](db).
	WhereEq("status", "active").
	OrderBy("created_at DESC").
	Limit(20).
	All(ctx)

err = orm.Insert(ctx, db, &user)

err = orm.UpdateFields(ctx, db, &user, "name", "status")

err = orm.DeleteByPK[User](ctx, db, 42)
```

И отдельно — relation-style API:

```go
users, err := orm.Query[User](db).
	Preload("Profile").
	Preload("Roles").
	All(ctx)
```
