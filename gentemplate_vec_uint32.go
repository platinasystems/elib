// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id Uint32 -d VecType=Uint32Vec -d Type=uint32 vec.tmpl]

// Copyright 2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elib

type Uint32Vec []uint32

func (p *Uint32Vec) Resize(n uint) {
	c := uint(cap(*p))
	l := uint(len(*p)) + n
	if l > c {
		c = NextResizeCap(l)
		q := make([]uint32, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *Uint32Vec) validate(new_len uint, zero uint32) *uint32 {
	c := uint(cap(*p))
	lʹ := uint(len(*p))
	l := new_len
	if l <= c {
		// Need to reslice to larger length?
		if l > lʹ {
			*p = (*p)[:l]
			for i := lʹ; i < l; i++ {
				(*p)[i] = zero
			}
		}
		return &(*p)[l-1]
	}
	return p.validateSlowPath(zero, c, l, lʹ)
}

func (p *Uint32Vec) validateSlowPath(zero uint32, c, l, lʹ uint) *uint32 {
	if l > c {
		cNext := NextResizeCap(l)
		q := make([]uint32, cNext, cNext)
		copy(q, *p)
		for i := c; i < cNext; i++ {
			q[i] = zero
		}
		*p = q[:l]
	}
	if l > lʹ {
		*p = (*p)[:l]
	}
	return &(*p)[l-1]
}

func (p *Uint32Vec) Validate(i uint) *uint32 {
	var zero uint32
	return p.validate(i+1, zero)
}

func (p *Uint32Vec) ValidateInit(i uint, zero uint32) *uint32 {
	return p.validate(i+1, zero)
}

func (p *Uint32Vec) ValidateLen(l uint) (v *uint32) {
	if l > 0 {
		var zero uint32
		v = p.validate(l, zero)
	}
	return
}

func (p *Uint32Vec) ValidateLenInit(l uint, zero uint32) (v *uint32) {
	if l > 0 {
		v = p.validate(l, zero)
	}
	return
}

func (p *Uint32Vec) ResetLen() {
	if *p != nil {
		*p = (*p)[:0]
	}
}

func (p Uint32Vec) Len() uint { return uint(len(p)) }
