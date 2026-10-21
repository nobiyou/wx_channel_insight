package media

import (
	"encoding/binary"
	"io"
)

const EncryptedPrefixSize uint64 = 128 * 1024

// DecryptReader matches wx_channel_hub's ISAAC64 video stream behavior. The
// platform encrypts only the first 128 KiB, so Range requests must seed the
// keystream at the requested byte offset.
type DecryptReader struct {
	reader       io.Reader
	ctx          *isaac64Context
	limit        uint64
	consumed     uint64
	keystream    [8]byte
	keystreamPos int
}

type isaac64Context struct {
	randrsl [256]uint64
	randcnt uint64
	mm      [256]uint64
	aa      uint64
	bb      uint64
	cc      uint64
}

func NewDecryptReader(reader io.Reader, key uint64, offset uint64, limit uint64) *DecryptReader {
	ctx := newIsaac64Context(key)
	dr := &DecryptReader{
		reader:       reader,
		ctx:          ctx,
		limit:        limit,
		keystreamPos: 8,
	}

	if limit == 0 {
		return dr
	}
	if offset >= limit {
		dr.consumed = limit
		return dr
	}

	dr.consumed = offset
	for i := uint64(0); i < offset/8; i++ {
		_ = dr.ctx.isaac64Random()
	}
	value := dr.ctx.isaac64Random()
	binary.BigEndian.PutUint64(dr.keystream[:], value)
	dr.keystreamPos = int(offset % 8)
	return dr
}

func (dr *DecryptReader) Read(p []byte) (int, error) {
	n, err := dr.reader.Read(p)
	if n <= 0 || dr.limit == 0 || dr.consumed >= dr.limit {
		return n, err
	}

	toDecrypt := uint64(n)
	if remaining := dr.limit - dr.consumed; toDecrypt > remaining {
		toDecrypt = remaining
	}
	for i := uint64(0); i < toDecrypt; i++ {
		if dr.keystreamPos >= len(dr.keystream) {
			value := dr.ctx.isaac64Random()
			binary.BigEndian.PutUint64(dr.keystream[:], value)
			dr.keystreamPos = 0
		}
		p[i] ^= dr.keystream[dr.keystreamPos]
		dr.keystreamPos++
	}
	dr.consumed += toDecrypt
	return n, err
}

func newIsaac64Context(seed uint64) *isaac64Context {
	ctx := &isaac64Context{}
	ctx.randrsl[0] = seed
	ctx.randinit(true)
	return ctx
}

func (ctx *isaac64Context) randinit(flag bool) {
	const golden = uint64(0x9e3779b97f4a7c13)
	a, b, c, d := golden, golden, golden, golden
	e, f, g, h := golden, golden, golden, golden

	for j := 0; j < 4; j++ {
		a, b, c, d, e, f, g, h = mix(a, b, c, d, e, f, g, h)
	}
	for j := 0; j < 256; j += 8 {
		if flag {
			a += ctx.randrsl[j]
			b += ctx.randrsl[j+1]
			c += ctx.randrsl[j+2]
			d += ctx.randrsl[j+3]
			e += ctx.randrsl[j+4]
			f += ctx.randrsl[j+5]
			g += ctx.randrsl[j+6]
			h += ctx.randrsl[j+7]
		}
		a, b, c, d, e, f, g, h = mix(a, b, c, d, e, f, g, h)
		ctx.mm[j], ctx.mm[j+1], ctx.mm[j+2], ctx.mm[j+3] = a, b, c, d
		ctx.mm[j+4], ctx.mm[j+5], ctx.mm[j+6], ctx.mm[j+7] = e, f, g, h
	}
	if flag {
		for j := 0; j < 256; j += 8 {
			a += ctx.mm[j]
			b += ctx.mm[j+1]
			c += ctx.mm[j+2]
			d += ctx.mm[j+3]
			e += ctx.mm[j+4]
			f += ctx.mm[j+5]
			g += ctx.mm[j+6]
			h += ctx.mm[j+7]
			a, b, c, d, e, f, g, h = mix(a, b, c, d, e, f, g, h)
			ctx.mm[j], ctx.mm[j+1], ctx.mm[j+2], ctx.mm[j+3] = a, b, c, d
			ctx.mm[j+4], ctx.mm[j+5], ctx.mm[j+6], ctx.mm[j+7] = e, f, g, h
		}
	}
	ctx.isaac64()
	ctx.randcnt = 256
}

func mix(a, b, c, d, e, f, g, h uint64) (uint64, uint64, uint64, uint64, uint64, uint64, uint64, uint64) {
	a -= e
	f ^= h >> 9
	h += a
	b -= f
	g ^= a << 9
	a += b
	c -= g
	h ^= b >> 23
	b += c
	d -= h
	a ^= c << 15
	c += d
	e -= a
	b ^= d >> 14
	d += e
	f -= b
	c ^= e << 20
	e += f
	g -= c
	d ^= f >> 17
	f += g
	h -= d
	e ^= g << 14
	g += h
	return a, b, c, d, e, f, g, h
}

func (ctx *isaac64Context) isaac64() {
	ctx.cc++
	ctx.bb += ctx.cc
	for j := 0; j < 256; j++ {
		x := ctx.mm[j]
		switch j % 4 {
		case 0:
			ctx.aa = ^(ctx.aa ^ (ctx.aa << 21))
		case 1:
			ctx.aa ^= ctx.aa >> 5
		case 2:
			ctx.aa ^= ctx.aa << 12
		case 3:
			ctx.aa ^= ctx.aa >> 33
		}
		ctx.aa += ctx.mm[(j+128)%256]
		y := ctx.mm[(x>>3)%256] + ctx.aa + ctx.bb
		ctx.mm[j] = y
		ctx.bb = ctx.mm[(y>>11)%256] + x
		ctx.randrsl[j] = ctx.bb
	}
}

func (ctx *isaac64Context) isaac64Random() uint64 {
	if ctx.randcnt == 0 {
		ctx.isaac64()
		ctx.randcnt = 256
	}
	ctx.randcnt--
	return ctx.randrsl[ctx.randcnt]
}
