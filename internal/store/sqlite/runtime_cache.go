package sqlite

import (
	"database/sql"
	"sync"
)

const maxPreparedStatementsPerPool = 64

type namespaceRuntime struct {
	mu              sync.RWMutex
	schemas         map[string]TableSchema
	objectTypes     map[string]string
	conflictTargets map[string]string
}

type poolRuntime struct {
	namespace *namespaceRuntime
	key       string

	mu         sync.Mutex
	statements map[string]*sql.Stmt
}

type namespaceRuntimeRef struct {
	runtime *namespaceRuntime
	refs    int
}

var (
	poolRuntimes       sync.Map
	namespaceRuntimes  = make(map[string]*namespaceRuntimeRef)
	namespaceRuntimeMu sync.Mutex
)

// RegisterNamespacePool attaches caches shared by all pools for one namespace.
func RegisterNamespacePool(key string, db *sql.DB) {
	if db == nil {
		return
	}

	namespaceRuntimeMu.Lock()
	ref := namespaceRuntimes[key]
	if ref == nil {
		ref = &namespaceRuntimeRef{
			runtime: &namespaceRuntime{
				schemas:         make(map[string]TableSchema),
				objectTypes:     make(map[string]string),
				conflictTargets: make(map[string]string),
			},
		}
		namespaceRuntimes[key] = ref
	}
	ref.refs++
	namespaceRuntimeMu.Unlock()

	poolRuntimes.Store(db, &poolRuntime{
		namespace:  ref.runtime,
		key:        key,
		statements: make(map[string]*sql.Stmt),
	})
}

// UnregisterNamespacePool closes cached statements and detaches one pool.
func UnregisterNamespacePool(db *sql.DB) {
	unregisterPool(db)
}

func unregisterPool(db *sql.DB) {
	if db == nil {
		return
	}
	value, ok := poolRuntimes.LoadAndDelete(db)
	if !ok {
		return
	}
	runtime := value.(*poolRuntime)
	runtime.mu.Lock()
	for key, stmt := range runtime.statements {
		_ = stmt.Close()
		delete(runtime.statements, key)
	}
	runtime.mu.Unlock()

	namespaceRuntimeMu.Lock()
	ref := namespaceRuntimes[runtime.key]
	if ref != nil {
		ref.refs--
		if ref.refs == 0 {
			delete(namespaceRuntimes, runtime.key)
		}
	}
	namespaceRuntimeMu.Unlock()
}

// InvalidateRuntime clears cached schema metadata after a schema mutation.
func InvalidateRuntime(db *sql.DB) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return
	}
	runtime.namespace.mu.Lock()
	clear(runtime.namespace.schemas)
	clear(runtime.namespace.objectTypes)
	clear(runtime.namespace.conflictTargets)
	runtime.namespace.mu.Unlock()
}

func cachedSchema(db *sql.DB, table string) (TableSchema, bool) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return TableSchema{}, false
	}
	runtime.namespace.mu.RLock()
	schema, ok := runtime.namespace.schemas[table]
	runtime.namespace.mu.RUnlock()
	return schema, ok
}

func cacheSchema(db *sql.DB, table string, schema TableSchema) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return
	}
	runtime.namespace.mu.Lock()
	runtime.namespace.schemas[table] = schema
	if schema.Type != "" {
		runtime.namespace.objectTypes[table] = schema.Type
	}
	runtime.namespace.mu.Unlock()
}

func cachedObjectType(db *sql.DB, name string) (string, bool) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return "", false
	}
	runtime.namespace.mu.RLock()
	objectType, ok := runtime.namespace.objectTypes[name]
	runtime.namespace.mu.RUnlock()
	return objectType, ok
}

func cacheObjectType(db *sql.DB, name, objectType string) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return
	}
	runtime.namespace.mu.Lock()
	runtime.namespace.objectTypes[name] = objectType
	runtime.namespace.mu.Unlock()
}

func cachedConflictTarget(db *sql.DB, table string) (string, bool) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return "", false
	}
	runtime.namespace.mu.RLock()
	target, ok := runtime.namespace.conflictTargets[table]
	runtime.namespace.mu.RUnlock()
	return target, ok
}

func cacheConflictTarget(db *sql.DB, table, target string) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		return
	}
	runtime.namespace.mu.Lock()
	runtime.namespace.conflictTargets[table] = target
	runtime.namespace.mu.Unlock()
}

func cachedStatement(db *sql.DB, key, query string) (*sql.Stmt, bool, error) {
	runtime, ok := runtimeForDB(db)
	if !ok {
		stmt, err := db.Prepare(query)
		return stmt, false, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if stmt, ok := runtime.statements[key]; ok {
		return stmt, true, nil
	}

	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, false, err
	}
	if len(runtime.statements) >= maxPreparedStatementsPerPool {
		return stmt, false, nil
	}
	runtime.statements[key] = stmt
	return stmt, true, nil
}

func runtimeForDB(db *sql.DB) (*poolRuntime, bool) {
	value, ok := poolRuntimes.Load(db)
	if !ok {
		return nil, false
	}
	return value.(*poolRuntime), true
}
