// Copyright © 2016-2019 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elib

type logger interface {
	Logf(format string, args ...interface{})
	Logln(args ...interface{})
}

type nolog struct{}

var Logger = logger(nolog{})

func (nolog) Logf(format string, args ...interface{}) {}

func (nolog) Logln(args ...interface{}) {}
