// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Hand-written (NOT generated; handfixes.json credkit-wiring,
// chrome-connect-through-kit): what the Chrome transport sends beyond the
// request itself goes through the credential kit. Its browser headers are
// judged with the request; its proxy CONNECT is one exchange of its own, which
// chromeProxyCONNECT sends through chromeConnectExchange.

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// judgeChromeExtras adds the browser headers req lacks, so the kit judges them
// with the request.
func judgeChromeExtras(req *http.Request) {
	if req.Header == nil {
		req.Header = http.Header{}
	}
	for _, h := range chromeHeaders {
		if req.Header.Get(h.name) == "" {
			req.Header.Set(h.name, h.value)
		}
	}
}

var errChromeProxyAnswer = errors.New("chrome transport: the proxy's answer to CONNECT is not valid HTTP")

// chromeConnectExchange writes a CONNECT to the proxy and reads its answer,
// inside the kit's guard: the kit judges the request before this writes it and
// holds what the proxy answers before the program sees it.
type chromeConnectExchange struct {
	conn net.Conn
	br   *bufio.Reader
}

func (x chromeConnectExchange) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Write(x.conn); err != nil {
		return nil, err
	}
	resp, err := http.ReadResponse(x.br, req)
	if err != nil {
		// A parse error quotes the answer, and with no response the kit has held
		// nothing in it. A network error quotes nothing the proxy wrote.
		if ne, ok := err.(net.Error); ok {
			return nil, ne
		}
		return nil, errChromeProxyAnswer
	}
	// Nothing reads a CONNECT answer's body, and after a 200 what follows is
	// the tunnel: the kit must not wait for it to end.
	_ = resp.Body.Close()
	resp.Body = http.NoBody
	return resp, nil
}
