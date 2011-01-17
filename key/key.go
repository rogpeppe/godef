package key

type MapKey interface {
	Hashcode() uint64
	Equals(m MapKey) bool
}

type Mapping struct {
	keys map[uint64] *entry
}

type entry struct {
	mkey MapKey
	key Key
	next *entry
}

type Key interface{}
type customKey bool

func IntKey(i int) Key {
	return i
}

func StringKey(s string) Key {
	return s
}

func NewMapping() *Mapping {
	return &Mapping{make(map[uint64] *entry)}
}

func (m *Mapping) Key(mkey MapKey) Key {
	h := mkey.Hashcode()
	if e, ok := m.keys[h]; ok {
		for ; e != nil; e = e.next {
			if e.mkey.Equals(mkey) {
				return e.key
			}
		}
	}
	k := new(customKey)
	m.keys[h] = &entry{mkey, k, m.keys[h]}
	return k
}
