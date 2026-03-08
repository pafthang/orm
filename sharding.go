package orm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type shardCtxKey struct{}

// WithShardKey stores shard key in context.
func WithShardKey(ctx context.Context, shardKey string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, shardCtxKey{}, shardKey)
}

func shardKeyFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v := ctx.Value(shardCtxKey{})
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// ShardResolver returns DB instance for provided shard key.
type ShardResolver interface {
	ResolveShard(ctx context.Context, shardKey string, info OperationInfo) (DB, error)
}

// RoutingPolicy configures router behavior for shard resolution.
type RoutingPolicy struct {
	RequireShard      bool
	RequireFor        map[Operation]bool
	UseTenantFallback bool
	TenantToShard     func(tenant any) (string, error)
}

// ShardReplica is a single DB node in shard cluster.
type ShardReplica struct {
	Name     string
	DB       DB
	Weight   int
	ReadOnly bool
	Healthy  bool
}

// ClusterResolvePolicy configures replica selection and fallback behavior.
type ClusterResolvePolicy struct {
	PreferReadReplicas     bool
	AllowUnhealthyFallback bool
	AllowWriteToReadOnly   bool
}

// ClusterResolver routes shard key to a weighted, health-aware replica.
type ClusterResolver struct {
	mu       sync.RWMutex
	shards   map[string][]ShardReplica
	counters sync.Map
	Policy   ClusterResolvePolicy
}

// NewClusterResolver creates a new resolver for shard orchestration.
func NewClusterResolver() *ClusterResolver {
	return &ClusterResolver{
		shards: make(map[string][]ShardReplica),
		Policy: ClusterResolvePolicy{
			PreferReadReplicas:     true,
			AllowUnhealthyFallback: true,
			AllowWriteToReadOnly:   false,
		},
	}
}

// Register adds or replaces replica in shard group.
func (r *ClusterResolver) Register(shardKey string, replica ShardReplica) error {
	if r == nil {
		return fmt.Errorf("resolver is nil")
	}
	if shardKey == "" {
		return fmt.Errorf("shard key is required")
	}
	if replica.DB == nil {
		return fmt.Errorf("replica db is nil")
	}
	if replica.Name == "" {
		replica.Name = shardKey
	}
	if replica.Weight <= 0 {
		replica.Weight = 1
	}
	if !replica.Healthy {
		replica.Healthy = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	reps := r.shards[shardKey]
	for i := range reps {
		if reps[i].Name == replica.Name {
			reps[i] = replica
			r.shards[shardKey] = reps
			return nil
		}
	}
	r.shards[shardKey] = append(reps, replica)
	return nil
}

// SetHealthy updates replica health flag.
func (r *ClusterResolver) SetHealthy(shardKey, replicaName string, healthy bool) error {
	if r == nil {
		return fmt.Errorf("resolver is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	reps := r.shards[shardKey]
	for i := range reps {
		if reps[i].Name == replicaName {
			reps[i].Healthy = healthy
			r.shards[shardKey] = reps
			return nil
		}
	}
	return fmt.Errorf("replica not found")
}

// ResolveShard resolves db node for shard key and operation.
func (r *ClusterResolver) ResolveShard(ctx context.Context, shardKey string, info OperationInfo) (DB, error) {
	_ = ctx
	if r == nil {
		return nil, fmt.Errorf("resolver is nil")
	}
	r.mu.RLock()
	replicas := append([]ShardReplica(nil), r.shards[shardKey]...)
	r.mu.RUnlock()
	if len(replicas) == 0 {
		return nil, fmt.Errorf("shard %q is not registered", shardKey)
	}
	candidates := r.filterByOperation(replicas, info.Operation)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no replicas available for operation %s", info.Operation)
	}
	healthy := make([]ShardReplica, 0, len(candidates))
	for _, rep := range candidates {
		if rep.Healthy {
			healthy = append(healthy, rep)
		}
	}
	if len(healthy) == 0 {
		if isReadOperation(info.Operation) && r.Policy.PreferReadReplicas {
			// For reads, prefer a healthy primary over unhealthy read replicas.
			writeCapable := make([]ShardReplica, 0, len(replicas))
			for _, rep := range replicas {
				if rep.ReadOnly {
					continue
				}
				if rep.Healthy {
					writeCapable = append(writeCapable, rep)
				}
			}
			if len(writeCapable) > 0 {
				healthy = writeCapable
			}
		}
	}
	if len(healthy) == 0 {
		if !r.Policy.AllowUnhealthyFallback {
			return nil, fmt.Errorf("no healthy replicas for shard %q", shardKey)
		}
		healthy = candidates
	}
	idx := weightedIndex(r.counterFor(shardKey, info.Operation), healthy)
	chosen := healthy[idx]
	if chosen.DB == nil {
		return nil, fmt.Errorf("chosen replica has nil db")
	}
	return chosen.DB, nil
}

func (r *ClusterResolver) filterByOperation(replicas []ShardReplica, op Operation) []ShardReplica {
	read := isReadOperation(op)
	candidates := make([]ShardReplica, 0, len(replicas))
	if read && r.Policy.PreferReadReplicas {
		readOnly := make([]ShardReplica, 0, len(replicas))
		for _, rep := range replicas {
			if rep.ReadOnly {
				readOnly = append(readOnly, rep)
			}
		}
		if len(readOnly) > 0 {
			return readOnly
		}
	}
	for _, rep := range replicas {
		if !read && rep.ReadOnly && !r.Policy.AllowWriteToReadOnly {
			continue
		}
		candidates = append(candidates, rep)
	}
	return candidates
}

func (r *ClusterResolver) counterFor(shardKey string, op Operation) *uint64 {
	key := fmt.Sprintf("%s|%s", shardKey, op)
	if v, ok := r.counters.Load(key); ok {
		return v.(*uint64)
	}
	var zero uint64
	actual, _ := r.counters.LoadOrStore(key, &zero)
	return actual.(*uint64)
}

func weightedIndex(counter *uint64, replicas []ShardReplica) int {
	if len(replicas) == 1 {
		return 0
	}
	total := 0
	for _, rep := range replicas {
		w := rep.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	if total <= 1 {
		return int(atomic.AddUint64(counter, 1)-1) % len(replicas)
	}
	n := int((atomic.AddUint64(counter, 1) - 1) % uint64(total))
	cur := 0
	for i, rep := range replicas {
		w := rep.Weight
		if w <= 0 {
			w = 1
		}
		cur += w
		if n < cur {
			return i
		}
	}
	return len(replicas) - 1
}

func isReadOperation(op Operation) bool {
	switch op {
	case OpByPK, OpExistsByPK, OpCount, OpQueryAll, OpQueryOne, OpQueryCount:
		return true
	default:
		return false
	}
}

// Router is a policy-driven sharding router.
type Router struct {
	Base     DB
	Resolver ShardResolver
	Policy   RoutingPolicy
}

// NewRouter creates a sharding router with sane defaults.
func NewRouter(base DB, resolver ShardResolver) *Router {
	return &Router{
		Base:     base,
		Resolver: resolver,
		Policy: RoutingPolicy{
			UseTenantFallback: true,
		},
	}
}

// SelectDB resolves shard DB from context and falls back to base DB.
func SelectDB(ctx context.Context, base DB, resolver ShardResolver, info OperationInfo) (DB, error) {
	r := NewRouter(base, resolver)
	return r.SelectDB(ctx, info)
}

// SelectDB resolves DB based on router policy and context shard/tenant info.
func (r *Router) SelectDB(ctx context.Context, info OperationInfo) (DB, error) {
	base := r.Base
	resolver := r.Resolver
	policy := r.Policy
	if base == nil {
		return nil, ErrInvalidQuery.with("select_db", info.Model, "", fmt.Errorf("base db is nil"))
	}
	if resolver == nil {
		if policy.Requires(info.Operation) {
			return nil, ErrInvalidQuery.with("select_db", info.Model, "", fmt.Errorf("shard is required for operation %s", info.Operation))
		}
		return base, nil
	}
	key, ok := shardKeyFromContext(ctx)
	if !ok && policy.UseTenantFallback {
		if tf, tok := tenantFromContext(ctx); tok {
			if policy.TenantToShard != nil {
				k, err := policy.TenantToShard(tf.Value)
				if err != nil {
					return nil, ErrInvalidQuery.with("select_db", info.Model, "", err)
				}
				key = k
			} else {
				key = fmt.Sprintf("%v", tf.Value)
			}
			ok = key != ""
		}
	}
	if !ok {
		if policy.Requires(info.Operation) {
			return nil, ErrInvalidQuery.with("select_db", info.Model, "", fmt.Errorf("shard key is required for operation %s", info.Operation))
		}
		return base, nil
	}
	db, err := resolver.ResolveShard(ctx, key, info)
	if err != nil {
		return nil, ErrInvalidQuery.with("select_db", info.Model, "", err)
	}
	if db == nil {
		return nil, ErrInvalidQuery.with("select_db", info.Model, "", fmt.Errorf("resolver returned nil db"))
	}
	return db, nil
}

func (p RoutingPolicy) Requires(op Operation) bool {
	if p.RequireFor != nil {
		if req, ok := p.RequireFor[op]; ok {
			return req
		}
	}
	return p.RequireShard
}

// InsertRouted resolves shard DB and then performs Insert.
func InsertRouted[T any](ctx context.Context, base DB, resolver ShardResolver, model *T) error {
	meta, _, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpInsert, Model: meta.Name, Table: meta.Table}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return err
	}
	return Insert(ctx, db, model)
}

// QueryRouted resolves shard DB and returns typed query builder.
func QueryRouted[T any](ctx context.Context, base DB, resolver ShardResolver) (*ModelQuery[T], error) {
	meta, err := Meta[T]()
	if err != nil {
		return nil, err
	}
	info := OperationInfo{Operation: OpQueryAll, Model: meta.Name, Table: meta.Table}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return nil, err
	}
	return Query[T](db), nil
}

func UpdateRouted[T any](ctx context.Context, base DB, resolver ShardResolver, model *T) error {
	meta, _, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpUpdate, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return err
	}
	return Update(ctx, db, model)
}

func UpdateFieldsRouted[T any](ctx context.Context, base DB, resolver ShardResolver, model *T, fields ...string) error {
	meta, _, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpUpdateFields, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return err
	}
	return UpdateFields(ctx, db, model, fields...)
}

func DeleteRouted[T any](ctx context.Context, base DB, resolver ShardResolver, model *T) error {
	meta, _, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpDelete, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return err
	}
	return Delete(ctx, db, model)
}

func ByPKRouted[T any](ctx context.Context, base DB, resolver ShardResolver, key any) (*T, error) {
	meta, err := Meta[T]()
	if err != nil {
		return nil, err
	}
	info := OperationInfo{Operation: OpByPK, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return nil, err
	}
	return ByPK[T](ctx, db, key)
}

func DeleteByPKRouted[T any](ctx context.Context, base DB, resolver ShardResolver, key any) error {
	meta, err := Meta[T]()
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpDeleteByPK, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return err
	}
	return DeleteByPK[T](ctx, db, key)
}

func ExistsByPKRouted[T any](ctx context.Context, base DB, resolver ShardResolver, key any) (bool, error) {
	meta, err := Meta[T]()
	if err != nil {
		return false, err
	}
	info := OperationInfo{Operation: OpExistsByPK, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return false, err
	}
	return ExistsByPK[T](ctx, db, key)
}

func CountRouted[T any](ctx context.Context, base DB, resolver ShardResolver) (int64, error) {
	meta, err := Meta[T]()
	if err != nil {
		return 0, err
	}
	info := OperationInfo{Operation: OpCount, Model: meta.Name, Table: meta.Table}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return 0, err
	}
	return Count[T](ctx, db)
}

func UpsertRouted[T any](ctx context.Context, base DB, resolver ShardResolver, model *T, opts ...UpsertOptions) error {
	meta, _, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpInsert, Model: meta.Name, Table: meta.Table}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return err
	}
	return Upsert(ctx, db, model, opts...)
}

func InsertBatchRouted[T any](ctx context.Context, base DB, resolver ShardResolver, rows []*T) (int64, error) {
	if len(rows) == 0 {
		return 0, ErrInvalidQuery.with("insert_batch_routed", "", "", fmt.Errorf("empty rows"))
	}
	meta, _, err := modelMetaAndValue(rows[0])
	if err != nil {
		return 0, err
	}
	info := OperationInfo{Operation: OpInsert, Model: meta.Name, Table: meta.Table}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return 0, err
	}
	return InsertBatch(ctx, db, rows)
}

func UpdateByPKRouted[T any](ctx context.Context, base DB, resolver ShardResolver, key any, fields map[string]any) (int64, error) {
	meta, err := Meta[T]()
	if err != nil {
		return 0, err
	}
	info := OperationInfo{Operation: OpUpdateFields, Model: meta.Name, Table: meta.Table, HasWhere: true}
	db, err := NewRouter(base, resolver).SelectDB(ctx, info)
	if err != nil {
		return 0, err
	}
	return UpdateByPK[T](ctx, db, key, fields)
}

// QueryWithRouter creates a routed query using router policy.
func QueryWithRouter[T any](ctx context.Context, r *Router) (*ModelQuery[T], error) {
	if r == nil {
		return nil, ErrInvalidQuery.with("query_with_router", "", "", fmt.Errorf("router is nil"))
	}
	meta, err := Meta[T]()
	if err != nil {
		return nil, err
	}
	db, err := r.SelectDB(ctx, OperationInfo{Operation: OpQueryAll, Model: meta.Name, Table: meta.Table})
	if err != nil {
		return nil, err
	}
	return Query[T](db), nil
}
