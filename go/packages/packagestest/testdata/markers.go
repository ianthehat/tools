// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake1

// The greek letters in this file mark points we use for marker tests.
// We use unique markers so we can make the tests stable against changes to
// this file.

const (
	_                 int = iota
	αSimpleMarker         //@αSimpleMarker
	offsetβMarker         //@mark(OffsetMarker, "β")
	regexγMarker          //@mark(RegexMarker, `\p{Greek}`)
	εMultipleζMarkers     //@εMultiple //@ζMarker
	useBeforeηDeclare     //@check(Declared, Declared) //@mark(Declared, "ηDeclare")
)

//Marker ι inside a comment //@mark(Comment,"ι")

func someFunc(a, b int) int {
	// The line below must be the first occurrence of the plus operator
	return a + b //@mark(NonIdentifier, "+")
}

// And some extra checks for interesting action parameters
//@check(αSimpleMarker, αSimpleMarker)
//@printI("StringAndInt", "Number %d", 12)
