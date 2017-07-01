package row

type Row struct {
	Idx    int
	Size   int
	Rsize  int
	Chars  []byte
	Render []byte
	Hl     []byte
	HlOpenComment bool
}
