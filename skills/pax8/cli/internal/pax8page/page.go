// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package pax8page is the one reading of how a Pax8 list pages. The Partner API
// documents every list as page (counting from 0, default 0) and size (default
// 10, maximum 200), answering
//
//	{"content":[...],"page":{"size":10,"totalElements":237,"totalPages":24,"number":0}}
//
// The block decides whether more rows follow. A row count never does: a server
// may answer fewer rows than asked for, and a full page may be the last one.
package pax8page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MaxSize is the documented maximum page size.
const MaxSize = 200

// maxWalkPages stops a walk a misbehaving server would otherwise never end.
const maxWalkPages = 100000

// Block is the page object of a list response.
type Block struct {
	Size          int
	TotalElements int
	TotalPages    int
	Number        int
}

// More reports whether a page follows this one.
func (b Block) More() bool {
	return b.Number+1 < b.TotalPages
}

// Read returns the page block of a list response, and false when the response
// has none or any of number, totalPages and totalElements is not a whole number
// of zero or more. size is informational and may be absent.
func Read(data []byte) (Block, bool) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var envelope struct {
		Page *struct {
			Size          json.RawMessage `json:"size"`
			TotalElements json.RawMessage `json:"totalElements"`
			TotalPages    json.RawMessage `json:"totalPages"`
			Number        json.RawMessage `json:"number"`
		} `json:"page"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Page == nil {
		return Block{}, false
	}
	number, ok1 := whole(envelope.Page.Number)
	pages, ok2 := whole(envelope.Page.TotalPages)
	total, ok3 := whole(envelope.Page.TotalElements)
	if !ok1 || !ok2 || !ok3 {
		return Block{}, false
	}
	size, _ := whole(envelope.Page.Size)
	return Block{Size: size, TotalElements: total, TotalPages: pages, Number: number}, true
}

// whole reads a non-negative whole number. The spec gives three of the four
// fields no type and its examples use numbers, so a numeric string and 3.0 read too.
func whole(raw json.RawMessage) (int, bool) {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(text); err == nil {
		return n, n >= 0
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || f < 0 || f > 1<<53 || f != float64(int64(f)) {
		return 0, false
	}
	return int(f), true
}

// RequestedPage reads the page parameter a request carried. An absent one is
// the documented default, page 0.
func RequestedPage(requested string) (int, error) {
	if requested == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(requested)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("page %q is not a page number", requested)
	}
	return n, nil
}

// Answers reports a block whose page is not the one requested, the sign of a
// server repeating one page however it is asked.
func (b Block) Answers(requested string) error {
	n, err := RequestedPage(requested)
	if err != nil {
		return err
	}
	if b.Number != n {
		return fmt.Errorf("asked for page %d and the API answered page %d", n, b.Number)
	}
	return nil
}

// Step is what one page of a walk says about the next.
type Step struct {
	// More is true when a page follows, and Next is its number.
	More bool
	Next string
}

// PageLimit is the row count a caller's "is this page short?" test should see:
// a page with more behind it is full and the last page is short, whatever the
// row count. The block has already decided, so the test can only agree.
func (s Step) PageLimit(rows int) int {
	if s.More {
		return rows
	}
	return rows + 1
}

// Walk checks one walk across a list. A walk is complete only when the block
// says the last page was read and the rows add up to the block's total, counting
// a walk that began on a later page as if the pages before it were full. The
// zero value is ready to use.
type Walk struct {
	started bool
	first   int
	rows    int
	pages   int
}

// Page records one page. requested is the page parameter the request carried,
// data the response and rows how many rows the response held. Every error means
// the reader cannot claim every row, and states only page numbers and counts.
func (w *Walk) Page(requested string, data []byte, rows int) (Step, error) {
	n, err := RequestedPage(requested)
	if err != nil {
		return Step{}, err
	}
	if !w.started {
		w.started, w.first = true, n
	}
	w.rows += rows
	w.pages++
	block, ok := Read(data)
	if !ok {
		return Step{}, fmt.Errorf("page %d has no page block, so the end of the list cannot be proven", n)
	}
	if err := block.Answers(requested); err != nil {
		return Step{}, err
	}
	if block.More() {
		if rows == 0 {
			return Step{}, fmt.Errorf("page %d of %d came back empty", n, block.TotalPages)
		}
		if w.pages >= maxWalkPages {
			return Step{}, fmt.Errorf("stopped after %d pages of a reported %d", w.pages, block.TotalPages)
		}
		return Step{More: true, Next: strconv.Itoa(n + 1)}, nil
	}
	if w.first == 0 || block.Size > 0 {
		if read := w.first*block.Size + w.rows; read != block.TotalElements {
			return Step{}, fmt.Errorf("read up to row %d and the API reports %d", read, block.TotalElements)
		}
	}
	return Step{}, nil
}
