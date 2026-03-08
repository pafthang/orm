# Sharding Guide

## Context Keys

Set shard key explicitly:

```go
ctx = orm.WithShardKey(ctx, "shard-1")
```

Tenant fallback is also supported via tenant context helpers.

## Resolver

Implement resolver:

```go
type Resolver struct{}

func (Resolver) ResolveShard(ctx context.Context, shardKey string, info orm.OperationInfo) (orm.DB, error) {
	return pickDB(shardKey), nil
}
```

For cluster orchestration use built-in weighted resolver:

```go
resolver := orm.NewClusterResolver()

_ = resolver.Register("shard-1", orm.ShardReplica{
	Name:   "primary",
	DB:     primaryDB,
	Weight: 1,
})
_ = resolver.Register("shard-1", orm.ShardReplica{
	Name:     "replica-a",
	DB:       replicaDB,
	Weight:   3,
	ReadOnly: true,
})
```

Health and failover:

```go
_ = resolver.SetHealthy("shard-1", "replica-a", false)
```

Policy knobs:
- `PreferReadReplicas`
- `AllowUnhealthyFallback`
- `AllowWriteToReadOnly`

## Routed Helpers

Available routed operations:
- insert/update/delete
- by-pk/delete-by-pk/exists/count
- upsert
- batch insert
- update-by-pk
- query entrypoint

Example:

```go
err := orm.InsertRouted(ctx, baseDB, resolver, &user)
q, err := orm.QueryRouted[User](ctx, baseDB, resolver)
rows, err := q.All(ctx)
```

## Policy-Driven Router

```go
router := orm.NewRouter(baseDB, resolver)
router.Policy.RequireFor = map[orm.Operation]bool{
	orm.OpByPK: true,
}
router.Policy.UseTenantFallback = true
```

Policy options:
- `RequireShard`
- `RequireFor` (operation-specific override)
- `UseTenantFallback`
- `TenantToShard` mapper

## Troubleshooting

- `shard key is required`: provide `WithShardKey` or enable tenant fallback.
- `resolver returned nil db`: check resolver map and default branch.
- `no healthy replicas`: set health flags or allow unhealthy fallback explicitly.
- inconsistent reads: confirm routing policy is identical across read/write paths.
