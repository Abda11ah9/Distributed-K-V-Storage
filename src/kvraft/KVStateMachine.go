package kvraft

// KVStateMachine defines the methods for a key-value state machine.
type KVStateMachine interface {
	Get(key string) (string, Err)
	Put(key, value string) Err
	Append(key, value string) Err
}

// MemoryKV implements an in-memory key-value store.
type MemoryKV struct {
	KV map[string]string
}

// NewMemoryKV initializes a new MemoryKV instance.
func NewMemoryKV() *MemoryKV {
	kvMap := make(map[string]string)
	return &MemoryKV{KV: kvMap}
}

// Get returns the value for a given key if it exists, otherwise returns ErrNoKey.
func (store *MemoryKV) Get(key string) (string, Err) {
	if val, found := store.KV[key]; found {
		return val, OK
	}
	return "", ErrNoKey
}

// Put sets the given key to the provided value.
func (store *MemoryKV) Put(key, value string) Err {
	store.KV[key] = value
	return OK
}

// Append concatenates the provided value to the existing value of the key.
func (store *MemoryKV) Append(key, value string) Err {
	store.KV[key] = store.KV[key] + value
	return OK
}
