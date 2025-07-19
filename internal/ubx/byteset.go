package ubx

type byteSet [4]uint64

func (s *byteSet) add(b byte) {
	s[b>>6] |= 1 << (b & 0b111111)
}

func (s *byteSet) contains(b byte) bool {
	return s[b>>6]&(1<<(b&0b111111)) != 0
}

func (s *byteSet) clear() {
	*s = byteSet{}
}
