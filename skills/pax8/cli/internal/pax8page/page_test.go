// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8page

import (
	"fmt"
	"strings"
	"testing"
)

func body(size, total, pages, number int) []byte {
	return []byte(fmt.Sprintf(`{"content":[],"page":{"size":%d,"totalElements":%d,"totalPages":%d,"number":%d}}`, size, total, pages, number))
}

func TestReadTheDocumentedBlock(t *testing.T) {
	b, ok := Read([]byte(`{"content":[{"id":"a"}],"page":{"size":10,"totalElements":109,"totalPages":11,"number":3}}`))
	if !ok || b != (Block{Size: 10, TotalElements: 109, TotalPages: 11, Number: 3}) {
		t.Fatalf("%+v %v", b, ok)
	}
	if !b.More() {
		t.Error("page 3 of 11 has more behind it")
	}
	// The spec gives number, totalPages and totalElements no type.
	if b, ok := Read([]byte(`{"page":{"size":"10","totalElements":"5","totalPages":1.0,"number":"0"}}`)); !ok || b.TotalElements != 5 || b.TotalPages != 1 || b.More() {
		t.Errorf("numeric strings and 1.0: %+v %v", b, ok)
	}
	if _, ok := Read([]byte("\xef\xbb\xbf" + `{"page":{"totalElements":0,"totalPages":0,"number":0}}`)); !ok {
		t.Error("a byte-order mark hid the block")
	}
}

func TestReadRefusesWhatIsNotABlock(t *testing.T) {
	for _, in := range []string{
		`[]`, `{}`, `not json`, `{"page":1}`, `{"page":null}`, `{"page":{}}`,
		`{"page":{"totalElements":1,"totalPages":1}}`,
		`{"page":{"totalElements":1,"number":0}}`,
		`{"page":{"totalPages":1,"number":0}}`,
		`{"page":{"totalElements":-1,"totalPages":1,"number":0}}`,
		`{"page":{"totalElements":1,"totalPages":1.5,"number":0}}`,
		`{"page":{"totalElements":"x","totalPages":1,"number":0}}`,
		`{"page":{"totalElements":null,"totalPages":1,"number":0}}`,
	} {
		if b, ok := Read([]byte(in)); ok {
			t.Errorf("%s read as %+v", in, b)
		}
	}
}

func TestWalkFollowsTheBlock(t *testing.T) {
	var w Walk
	for i, want := range []struct {
		requested string
		rows      int
		more      bool
		next      string
	}{{"", 200, true, "1"}, {"1", 200, true, "2"}, {"2", 37, false, ""}} {
		step, err := w.Page(want.requested, body(200, 437, 3, i), want.rows)
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		if step.More != want.more || step.Next != want.next {
			t.Errorf("page %d: %+v", i, step)
		}
	}
}

func TestWalkEndsOnAFullLastPageAndOnAnEmptyList(t *testing.T) {
	var full Walk
	if step, err := full.Page("0", body(200, 200, 1, 0), 200); err != nil || step.More {
		t.Errorf("a full last page: %+v %v", step, err)
	}
	var empty Walk
	if step, err := empty.Page("0", body(200, 0, 0, 0), 0); err != nil || step.More {
		t.Errorf("an empty list: %+v %v", step, err)
	}
}

func TestWalkFollowsAShortPageWithMoreBehindIt(t *testing.T) {
	var w Walk
	step, err := w.Page("0", body(50, 137, 3, 0), 50)
	if err != nil || !step.More {
		t.Fatalf("%+v %v", step, err)
	}
	if got := step.PageLimit(50); got != 50 {
		t.Errorf("a page with more behind it reads as full, got limit %d", got)
	}
	if got := (Step{}).PageLimit(50); got <= 50 {
		t.Errorf("the last page reads as short, got limit %d", got)
	}
}

func TestWalkFailsWhatCannotBeProven(t *testing.T) {
	for name, run := range map[string]func(w *Walk) error{
		"no block": func(w *Walk) error {
			_, err := w.Page("0", []byte(`{"content":[{"id":"a"}]}`), 1)
			return err
		},
		"the wrong page answered": func(w *Walk) error {
			_, err := w.Page("1", body(10, 25, 3, 0), 10)
			return err
		},
		"empty before the last page": func(w *Walk) error {
			_, err := w.Page("0", body(10, 25, 3, 0), 0)
			return err
		},
		"one page claimed, rows missing": func(w *Walk) error {
			_, err := w.Page("0", body(10, 25, 1, 0), 10)
			return err
		},
		"not a page number": func(w *Walk) error {
			_, err := w.Page("next", body(10, 25, 3, 0), 10)
			return err
		},
		"a negative page": func(w *Walk) error {
			_, err := w.Page("-1", body(10, 25, 3, 0), 10)
			return err
		},
		"rows short across the walk": func(w *Walk) error {
			if _, err := w.Page("0", body(10, 25, 3, 0), 10); err != nil {
				return nil
			}
			if _, err := w.Page("1", body(10, 25, 3, 1), 1); err != nil {
				return nil
			}
			_, err := w.Page("2", body(10, 25, 3, 2), 5)
			return err
		},
		"a server that never stops": func(w *Walk) error {
			w.pages = maxWalkPages
			_, err := w.Page("0", body(10, 25, 3, 0), 10)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			var w Walk
			err := run(&w)
			if err == nil {
				t.Fatal("the walk passed")
			}
			if strings.Contains(err.Error(), "id") && strings.Contains(err.Error(), `"a"`) {
				t.Errorf("an error repeated the response: %v", err)
			}
		})
	}
}

// A walk resumed part way counts the pages before it as full.
func TestWalkResumedFromALaterPage(t *testing.T) {
	var w Walk
	if _, err := w.Page("2", body(10, 25, 3, 2), 5); err != nil {
		t.Fatalf("a resumed walk: %v", err)
	}
	var short Walk
	if _, err := short.Page("2", body(10, 25, 3, 2), 4); err == nil {
		t.Error("a last page one row short passed on a resumed walk")
	}
	var gone Walk
	if _, err := gone.Page("5", body(10, 25, 3, 5), 0); err == nil {
		t.Error("a resume past the end of a shrunken list passed")
	}
}

func TestAnswersReadsTheDefaultAsPageZero(t *testing.T) {
	b := Block{Number: 0, TotalPages: 3}
	if err := b.Answers(""); err != nil {
		t.Error(err)
	}
	if err := b.Answers("1"); err == nil {
		t.Error("page 0 answered for page 1 passed")
	}
}
