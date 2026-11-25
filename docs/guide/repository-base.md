# `repository/base.go`

`repository/base.go` contains shared database helpers for the backend's repository layer. It wraps GORM's `*gorm.DB` with generic entity and filter types, and provides context-aware database access plus common insert operations.

## `BaseRepository[T, F]`

`T` is the entity type handled by a repository; `F` is its filter type. The base stores the application's GORM database handle:

```go
base := repository.NewBaseRepository[models.AuditLog, models.AuditLogFilter](db)
```

Concrete repositories typically embed or hold a `BaseRepository` so their methods can reuse its database and transaction helpers. The repository instance is normally created during application setup and reused; callers pass request context to individual operations rather than creating a repository for each query.

## Choosing a database handle

`getDB(ctx)` looks for a `*gorm.DB` transaction stored under `TxContextKey` in the context. If it finds one, it returns that transaction. Otherwise, it returns the base DB with the context attached via GORM's `WithContext`.

`getDBForWrite(ctx)` follows the same transaction lookup for writes, but starts a transaction with `Begin()` when the context has none. Its boolean result tells the caller whether it started that transaction:

- `false`: the caller is already inside a transaction, so this method must not commit or roll it back.
- `true`: this method started a transaction, so the write method commits on success and rolls back on failure.

This lets standalone writes manage their own transaction while allowing writes inside `WithTransaction` to participate in one shared transaction. An explicit transaction around one SQL statement is often unnecessary for atomicity, because a single statement is already atomic; the current implementation nevertheless wraps `Save` and `SaveBatch` this way, which also gives it a place to report commit errors.

## Insert methods

### `Save`

`Save(ctx, entity)` calls GORM's `Create` to insert one entity. If it started a transaction, it rolls back when the insert fails and commits otherwise. If the context already carries a transaction, it leaves commit/rollback to the outer transaction owner.

### `SaveBatch`

`SaveBatch(ctx, entities)` returns immediately for an empty slice. Otherwise it calls GORM's `CreateInBatches(entities, 1000)`, which splits the input into insert batches of at most 1,000 records. It applies the same transaction ownership behavior as `Save`.

`CreateInBatches` is part of GORM (`gorm.io/gorm`), not defined in this project. The batch size of 1,000 is the value chosen by this code; this file does not document the reason for that exact limit.

## `WithTransaction`

Use `WithTransaction(ctx, db, fn)` when multiple repository operations must commit or roll back together:

1. It begins a GORM transaction.
2. It puts the transaction in a derived context under `TxContextKey`.
3. It calls `fn` with that context. Repository methods must receive and use this context to join the transaction.
4. It rolls back if the callback returns an error, or commits if it succeeds. A panic causes a rollback and is converted to an error.

For example:

```go
err := repository.WithTransaction(ctx, db, func(txCtx context.Context) error {
	if err := accounts.Save(txCtx, account); err != nil {
		return err
	}
	return auditLogs.Save(txCtx, auditLog)
})
```

Both saves use the same transaction. If either returns an error, neither write is committed. Calls made without `WithTransaction` use their own write transaction through `getDBForWrite`.

## Filtering methods

Generic `ByFilter`, `Count`, and reflection-based `applyFilter` implementations remain in this file only as commented-out code. Active filtering is implemented by concrete repositories, which can express model-specific fields, joins, and conditions in their own `applyFilter`, `ByFilter`, and `Count` methods. For example, see `repository/audit_log_repository.go`.
