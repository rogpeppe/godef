package audio

type Format struct {
	NumChans int		// number of channels (0 if unset)
	Rate int 			// samples per second (0 if unset)
	Layout int
	Type int
}

type Formatted interface {
	GetFormat(name string) Format
}

type FormatSetter interface {
	SetFormat(f Format)
}

const Unspecified = 0

// layouts (earlier are considered better)
const (
	Interleaved = iota + 1
	NonInterleaved
	Mono
)

// types (earlier are considered better)
const (
	Float32Type = iota + 1
	Int16Type
)

func (f0 Format) Eq(f1 Format) bool {
	return f0.NumChans == f1.NumChans &&
		f0.Rate == f1.Rate &&
		f0.Layout == f1.Layout &&
		f0.Type == f1.Type
}

func (f Format) GetFormat(_ string) Format {
	return f
}

func(f0 Format) Match(f1 Format) bool {
	return match(f0.NumChans, f1.NumChans) &&
		match(f0.Rate, f1.Rate) &&
		match(f0.Layout, f1.Layout) &&
		match(f0.Type, f1.Type)
}

func (f Format) FullySpecified() bool {
	return f.NumChans != Unspecified &&
			f.Rate != Unspecified &&
			f.Layout != Unspecified &&
			f.Type != Unspecified
}

func (f Format) AllocBuffer(n int) Buffer {
	switch f.Type {
	case Float32Type:
		switch f.Layout {
		case Interleaved:
			return AllocNFloat32Buf(f.NumChans, n)
		case NonInterleaved:
			return AllocFloat32NBuf(f.NumChans, n)
		case Mono:
			return make(Float32Buf, n)
		}
	case Int16Type:
		if f.NumChans == 1 && f.Layout == Mono {
			return make(Int16Buf, n)
		}
	}
	panic("AllocBuffer on invalid format")
}

// set all the unspecified fields in f0 to
// values taken from f1.
func (f0 Format) Combine(f1 Format) Format {
	if f0.NumChans == Unspecified {
		f0.NumChans = f1.NumChans
	}
	if f0.Rate == Unspecified {
		f0.Rate = f1.Rate
	}
	if f0.Layout == Unspecified {
		f0.Layout = f1.Layout
	}
	if f0.Type == Unspecified {
		f0.Type = f1.Type
	}
	return f0
}

func (f0 Format) CombineBest(f1 Format) Format {
	if f0.NumChans < f1.NumChans {
		f0.NumChans = f1.NumChans
	}
	if f0.Rate < f1.Rate {
		f0.Rate = f1.Rate
	}
	if f0.Layout < f1.Layout {
		f0.Layout = f1.Layout
	}
	if f0.Type < f1.Type {
		f0.Type = f1.Type
	}
	return f0
}

func (f Format) TimeToSamples(t Time) int64 {
	if t.real {
		if f.Rate == 0 {
			panic("unspecified rate")
		}
		return t.t * int64(f.Rate) / 1e9
	}
	return t.t
}

func match(a, b int) bool {
	return a == b || a == Unspecified || b == Unspecified
}
