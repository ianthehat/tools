// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packagestest_test

import (
	"bytes"
	"fmt"
	"go/token"
	"io/ioutil"
	"log"
	"testing"

	"golang.org/x/tools/go/packages/packagestest"
)

func TestMarker(t *testing.T) {
	const filename = "testdata/markers.go"
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Could not read test file %v: %v", filename, err)
	}

	expectAnchors := map[string]string{
		"αSimpleMarker": "α",
		"OffsetMarker":  "β",
		"RegexMarker":   "γ",
		"εMultiple":     "ε",
		"ζMarker":       "ζ",
		"Declared":      "η",
		"Comment":       "ι",
		"NonIdentifier": "+",
	}
	expectChecks := map[string]string{
		"Declared":      "Declared",
		"αSimpleMarker": "αSimpleMarker",
	}
	expectPrints := map[string]string{
		"StringAndInt": "Number 12",
	}

	markers := packagestest.Markers{}
	if err := markers.Extract(filename, nil); err != nil {
		t.Fatalf("Failed to extract markers: %v", err)
	}
	anchors := markers.Anchors(t)
	checks := make(map[string]packagestest.Position, len(expectChecks))
	prints := make(map[string]string, len(expectPrints))
	markers.Invoke(t, map[string]interface{}{
		"check": func(name string, pos packagestest.Position) {
			checks[name] = pos
		},
		"printI": func(name string, format string, value int) {
			prints[name] = fmt.Sprintf(format, value)
		},
	})
	if len(anchors) != len(expectAnchors) {
		t.Fatalf("Got %d anchors expected %d", len(anchors), len(expectAnchors))
	}
	for name, tok := range expectAnchors {
		offset := bytes.Index(content, []byte(tok))
		before := []byte(content)[:offset]
		line := bytes.Count(before, []byte("\n")) + 1
		start := 0
		if line > 1 {
			start = bytes.LastIndex(before, []byte("\n"))
		}
		expect := packagestest.Position{
			Offset: offset,
			Position: token.Position{
				Filename: filename,
				Line:     line,
				Column:   len(string(before[start:])),
			},
		}
		got, found := anchors[name]
		if !found {
			t.Errorf("Expected anchor %s is missing", name)
		}
		if got != expect {
			t.Errorf("For %s got %s expected %s", name, got, expect)
		}
	}
	if len(checks) != len(expectChecks) {
		t.Fatalf("Got %d checks expected %d", len(checks), len(expectChecks))
	}
	for name, anchor := range expectChecks {
		got, found := checks[name]
		if !found {
			t.Errorf("Expected check %s is missing", name)
			continue
		}
		expect, found := anchors[anchor]
		if !found {
			t.Errorf("Expected anchor %s is missing", anchor)
			continue
		}
		if got != expect {
			t.Errorf("For %s got %s expected %s", name, got, expect)
		}
	}
	if len(prints) != len(expectPrints) {
		t.Fatalf("Got %d prints expected %d", len(prints), len(expectPrints))
	}
	for name, expect := range expectPrints {
		got, found := prints[name]
		if !found {
			t.Errorf("Expected print %s is missing", name)
			log.Printf("%+#v", prints)
			continue
		}
		if got != expect {
			t.Errorf("For %s got %s expected %s", name, got, expect)
		}
	}
}
