// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmarks_test

func BenchmarkImports(b *testing.B) {
	// Prepare our scratch directory to run the tests in
	// force the module cache up to date
	// sub run each of the imports tests
	// run each imports set function b.N times
	for n := 0; n < b.N; n++ {
		Fib(10)
	}
}
