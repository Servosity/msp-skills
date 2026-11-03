// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"encoding/binary"
	"math"
	mbits "math/bits"
)

// deflateCanStart reports whether a raw deflate stream starting at b[0] can
// decode at least want bytes before it ends or fails. It reads only the
// first block's header and, for a fixed-Huffman block, its first symbols, so
// the offsets that cannot start a stream (nearly all of them) are passed
// over without running the decompressor. It never rejects an offset the
// decompressor would decode want bytes from.
func deflateCanStart(b []byte, want int) bool {
	br := bitReader{b: b}
	final, ok := br.bits(1)
	if !ok {
		return false
	}
	btype, _ := br.bits(2)
	switch btype {
	case 0:
		// Stored: byte-aligned LEN and its complement.
		return len(b) >= 5 && b[1] == ^b[3] && b[2] == ^b[4] // exact: a stored header is 5 bytes; a shorter one is no stream
	case 3:
		return false
	case 2:
		// Dynamic: the code-length code must be a complete prefix code, or
		// the decompressor rejects the block.
		hlit, ok1 := br.bits(5)
		_, ok2 := br.bits(5)
		hclen, ok3 := br.bits(4)
		if !ok1 || !ok2 || !ok3 || hlit > 29 {
			return false
		}
		left, count := 1<<7, 0
		for i := 0; i < int(hclen)+4; i++ {
			l, ok := br.bits(3)
			if !ok {
				return false
			}
			if l > 0 {
				left -= 1 << (7 - l)
				count++
			}
		}
		return left == 0 || count == 1 && left == 1<<6
	}
	// Fixed Huffman: decode symbols until want bytes are out, the block
	// ends, or a code is invalid.
	out := 0
	for out < want {
		code, ok := br.huff(7)
		if !ok {
			return false
		}
		var sym int
		switch {
		case code <= 23:
			sym = 256 + int(code)
		default:
			c8, ok := br.extend(code)
			if !ok {
				return false
			}
			switch {
			case c8 >= 48 && c8 <= 191:
				sym = int(c8 - 48)
			case c8 >= 192 && c8 <= 199:
				sym = 280 + int(c8-192)
			default:
				c9, ok := br.extend(c8)
				if !ok || c9 < 400 {
					return false
				}
				sym = 144 + int(c9-400)
			}
		}
		switch {
		case sym < 256:
			out++
		case sym == 256:
			// End of block: a later block may go on.
			return final == 0
		case sym > 285:
			return false
		default:
			length, ok := br.lengthOf(sym)
			if !ok {
				return false
			}
			d, ok := br.huff(5)
			if !ok || d > 29 {
				return false
			}
			dist, ok := br.distOf(int(d))
			if !ok || dist > out {
				return false
			}
			out += length
		}
	}
	return true
}

type bitReader struct {
	b   []byte
	pos int // in bits
}

func (r *bitReader) bits(n int) (uint32, bool) {
	if r.pos+n > len(r.b)*8 {
		return 0, false
	}
	i := r.pos >> 3
	if i+4 <= len(r.b) {
		v := binary.LittleEndian.Uint32(r.b[i:]) >> (r.pos & 7) & (1<<n - 1)
		r.pos += n
		return v, true
	}
	v := uint64(0)
	for k := min(i+4, len(r.b)) - 1; k >= i; k-- { // n <= 25 bits: four bytes hold them
		v = v<<8 | uint64(r.b[k])
	}
	v = v >> (r.pos & 7) & (1<<n - 1)
	r.pos += n
	return uint32(v & math.MaxUint32), true
}

// huff reads n bits most significant first, as Huffman codes are packed.
func (r *bitReader) huff(n int) (uint32, bool) {
	v, ok := r.bits(n)
	return mbits.Reverse32(v) >> (32 - n), ok
}

func (r *bitReader) extend(code uint32) (uint32, bool) {
	bit, ok := r.bits(1)
	return code<<1 | bit, ok
}

var (
	lenBase  = [29]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
	lenExtra = [29]int{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
)

func (r *bitReader) lengthOf(sym int) (int, bool) {
	i := sym - 257
	e, ok := r.bits(lenExtra[i])
	return lenBase[i] + int(e), ok
}

func (r *bitReader) distOf(d int) (int, bool) {
	if d < 4 {
		return d + 1, true
	}
	extra := d/2 - 1
	e, ok := r.bits(extra)
	return (2+d&1)<<extra + 1 + int(e), ok
}

// fixedSym reads one symbol of the fixed Huffman code: from a table of
// every next nine bits, while four bytes are left.
func (r *bitReader) fixedSym() (int, bool) {
	if i := r.pos >> 3; i+4 <= len(r.b) {
		e := fixedTab[binary.LittleEndian.Uint32(r.b[i:])>>(r.pos&7)&511]
		r.pos += int(e & 15)
		return int(e >> 4), true
	}
	return r.fixedSymBits()
}

// fixedTab is the symbol and code length for each next nine bits.
var fixedTab = func() (t [512]uint16) {
	for i := range t {
		br := bitReader{b: []byte{byte(i), byte(i >> 8)}}
		sym, _ := br.fixedSymBits()
		t[i] = uint16(sym)<<4 | uint16(br.pos)
	}
	return t
}()

func (r *bitReader) fixedSymBits() (int, bool) {
	code, ok := r.huff(7)
	if !ok {
		return 0, false
	}
	if code <= 23 {
		return 256 + int(code), true
	}
	c8, ok := r.extend(code)
	switch {
	case !ok:
		return 0, false
	case c8 >= 48 && c8 <= 191:
		return int(c8 - 48), true
	case c8 >= 192 && c8 <= 199:
		return 280 + int(c8-192), true
	}
	c9, ok := r.extend(c8)
	if !ok || c9 < 400 {
		return 0, false
	}
	return 144 + int(c9-400), true
}

type inflateEnd int

const (
	inflateBroken inflateEnd = iota // an invalid code, or the input ran out
	inflateDone                     // the final block ended
	inflateOther                    // a stored or dynamic block: the decompressor's job
	inflateLimit                    // the output reached limit
)

// inflateFixed decodes a raw deflate stream at b[0] while its blocks use the
// fixed Huffman code, appending to out, and says how it stopped and how
// many bytes of b it read. With stop >= 0 it stops (inflateBroken) at the
// symbol boundary at bit stop.
func inflateFixed(b, out []byte, limit, stop int) ([]byte, inflateEnd, int) {
	br := bitReader{b: b}
	o, e := inflateFixedBits(&br, out, limit, stop)
	return o, e, (br.pos + 7) / 8
}

func inflateFixedBits(br *bitReader, out []byte, limit, stop int) ([]byte, inflateEnd) {
	start := len(out)
	for {
		final, ok := br.bits(1)
		if !ok {
			return out, inflateBroken
		}
		btype, ok := br.bits(2)
		switch {
		case !ok || btype == 3:
			return out, inflateBroken
		case btype != 1:
			return out, inflateOther
		}
		for {
			if br.pos == stop {
				return out, inflateBroken
			}
			sym, ok := br.fixedSym()
			if !ok || sym > 285 {
				return out, inflateBroken
			}
			if sym < 256 {
				if len(out)-start >= limit {
					return out, inflateLimit
				}
				out = append(out, byte(sym&0xff))
				continue
			}
			if sym == 256 {
				break
			}
			length, ok := br.lengthOf(sym)
			if !ok {
				return out, inflateBroken
			}
			d, ok := br.huff(5)
			if !ok || d > 29 {
				return out, inflateBroken
			}
			dist, ok := br.distOf(int(d))
			if !ok || dist > len(out)-start {
				return out, inflateBroken
			}
			if len(out)-start+length > limit {
				return out, inflateLimit
			}
			for k := 0; k < length; k++ {
				out = append(out, out[len(out)-dist])
			}
		}
		if final == 1 {
			return out, inflateDone
		}
	}
}

const (
	inflateRedo inflateEnd = 100 // joined an attempt that ended: decode it again
	inflateJoin inflateEnd = 101 // joined an attempt that broke: it breaks no later
)

// deflateMemo lets a raw deflate attempt take an earlier attempt's outcome.
// Two attempts at one symbol boundary in blocks of one kind decode the same
// symbols from there on; a back-reference is valid when its distance is at
// most the output so far, so an attempt with no more output than the
// earlier one had there fails no later than it did. The memo is a cache
// (an entry can be overwritten, which only loses a join), and an attempt
// uses it only after memoAfter symbols: at a random offset an attempt fails
// sooner, so only a long stream (a real one, or one repeated through a
// part, read from each of its offsets) pays for it.
type deflateMemo struct {
	cache [1 << 16]memoEntry
	gen   uint32
	done  []bool   // per numbered attempt: its stream ended
	outs  [][]byte // per numbered attempt that ended: what it decoded
	from  int32    // the attempt an inflateRedo or inflateJoin joined
	// where it joined: the output each attempt had there, and the bit
	joinA, joinB, joinBit int
	id                    int32 // the current attempt's number, or -1
	used                  int   // the bytes the last fixedEnds read
	n                     int   // the output it counted
}

type memoEntry struct {
	gen uint32
	att int32
	key uint64 // part offset in bits * 2 + final
	out uint64
}

const memoAfter = 64

func (m *deflateMemo) reset() {
	m.gen++
	m.done, m.outs = m.done[:0], m.outs[:0]
}

// fixedEnds decodes, without producing output, the raw deflate stream at
// offset base of p while its blocks use the fixed Huffman code.
// The attempt is numbered (m.id) when it first uses the memo.
func (m *deflateMemo) fixedEnds(p []byte, base int) inflateEnd {
	m.id = -1
	br := bitReader{b: p[base:]}
	out, syms := 0, 0
	defer func() { m.used, m.n = (br.pos+7)/8, out }()
	for {
		final, ok := br.bits(1)
		if !ok {
			return inflateBroken
		}
		btype, ok := br.bits(2)
		switch {
		case !ok || btype == 3:
			return inflateBroken
		case btype != 1:
			return inflateOther
		}
		for {
			if syms++; syms > memoAfter {
				if m.id < 0 {
					m.id = i32(len(m.done))
					m.done, m.outs = append(m.done, false), append(m.outs, nil)
				}
				key := u64(base*8+br.pos)*2 + uint64(final)
				e := &m.cache[key*0x9E3779B97F4A7C15>>48]
				if e.gen == m.gen && e.key == key && u64(out) <= e.out {
					m.from, m.joinA, m.joinB, m.joinBit = e.att, toInt(e.out), out, br.pos
					if m.done[e.att] {
						return inflateRedo
					}
					return inflateJoin
				}
				*e = memoEntry{gen: m.gen, att: m.id, key: key, out: u64(out)}
			}
			sym, ok := br.fixedSym()
			if !ok || sym > 285 {
				return inflateBroken
			}
			if sym < 256 {
				out++
				continue
			}
			if sym == 256 {
				break
			}
			length, ok := br.lengthOf(sym)
			if !ok {
				return inflateBroken
			}
			d, ok := br.huff(5)
			if !ok || d > 29 {
				return inflateBroken
			}
			dist, ok := br.distOf(int(d))
			if !ok || dist > out {
				return inflateBroken
			}
			out += length
		}
		if final == 1 {
			if m.id >= 0 {
				m.done[m.id] = true
			}
			return inflateDone
		}
	}
}
