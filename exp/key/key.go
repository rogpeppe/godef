package key

// Mapping holds a set of unique keys corresponding
// to Hasher values.
type Mapping struct {
	keys map[uint64]*entry
}

// Hasher represents a value that can be used as a map key.
type Hasher interface {
	Hashcode() uint64
	Equals(m Hasher) bool
}

type entry struct {
	mkey Hasher
	key  Key
	next *entry
}

// Key represents a comparable value.
type Key interface{}

type customKey struct {
	key Hasher
}

// NewMapping creates a new Mapping object
func NewMapping() *Mapping {
	return &Mapping{make(map[uint64]*entry)}
}

// Key returns a comparable Key value corresponding
// to mkey. If the same mkey is passed twice to
// the same Mapping, the same Key will be returned.
func (m *Mapping) Key(mkey Hasher) Key {
	h := mkey.Hashcode()
	if e, ok := m.keys[h]; ok {
		for ; e != nil; e = e.next {
			if e.mkey.Equals(mkey) {
				return e.key
			}
		}
	}
	k := &customKey{mkey}
	m.keys[h] = &entry{mkey, k, m.keys[h]}
	return k
}

// Original returns the original key value for a given Key,
// if it was created with the Key function;
// otherwise it returns nil.
func (m *Mapping) Original(k Key) Hasher {
	if k, ok := k.(*customKey); ok {
		return k.key
	}
	return nil
}
