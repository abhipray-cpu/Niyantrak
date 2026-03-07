package backend

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// Envelope wraps a value with its type name so JSON roundtrips preserve the concrete Go type.
// Redis and PostgreSQL backends use this to serialize/deserialize algorithm state correctly.
type Envelope struct {
	Type string          `json:"_type"`
	Data json.RawMessage `json:"data"`
}

// typeRegistry maps type names to their reflect.Type so we can instantiate the
// correct concrete struct when unmarshalling.
var (
	registryMu   sync.RWMutex
	typeRegistry = map[string]reflect.Type{}
)

// RegisterType registers a concrete type so that JSON-backed backends can
// reconstruct it on Get(). Typically called in an init() block.
//
//	backend.RegisterType((*algorithm.TokenBucketState)(nil))
func RegisterType(ptr interface{}) {
	t := reflect.TypeOf(ptr)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := t.PkgPath() + "." + t.Name()

	registryMu.Lock()
	typeRegistry[name] = t
	registryMu.Unlock()
}

// Wrap creates an Envelope for value, encoding its type name and JSON payload.
// For primitive types (string, int64, etc.) it returns the raw JSON bytes directly
// without an envelope, so IncrementAndGet values remain simple.
func Wrap(value interface{}) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}

	t := reflect.TypeOf(value)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Only wrap registered struct types in an envelope.
	name := t.PkgPath() + "." + t.Name()

	registryMu.RLock()
	_, registered := typeRegistry[name]
	registryMu.RUnlock()

	if !registered {
		// Primitive or unregistered type — marshal directly.
		return json.Marshal(value)
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	env := Envelope{
		Type: name,
		Data: data,
	}
	return json.Marshal(env)
}

// Unwrap decodes bytes produced by Wrap back into the original concrete type.
// If the bytes don't look like an envelope (no _type field), they are unmarshalled
// into a plain interface{} as a fallback.
func Unwrap(raw []byte) (interface{}, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Probe for envelope structure. If raw is not a JSON object with a "_type"
	// field, treat it as a plain value (not an envelope).
	var probe struct {
		Type string `json:"_type"`
	}
	isEnvelope := json.Unmarshal(raw, &probe) == nil && probe.Type != ""
	if !isEnvelope {
		// Not an envelope — return as plain value.
		var v interface{}
		if json.Unmarshal(raw, &v) != nil {
			// Not valid JSON — return the raw bytes as a string.
			return string(raw), nil
		}
		return v, nil
	}

	// It's an envelope — look up the registered type.
	registryMu.RLock()
	t, ok := typeRegistry[probe.Type]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("backend: unregistered type %q — call backend.RegisterType", probe.Type)
	}

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("backend: unmarshal envelope: %w", err)
	}

	ptr := reflect.New(t).Interface()
	if err := json.Unmarshal(env.Data, ptr); err != nil {
		return nil, fmt.Errorf("backend: unmarshal data for %s: %w", probe.Type, err)
	}

	return ptr, nil
}
