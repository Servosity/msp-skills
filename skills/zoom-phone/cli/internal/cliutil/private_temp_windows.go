//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

// securityAttributesSize is sizeof(SECURITY_ATTRIBUTES): a DWORD, a pointer and
// a BOOL, padded to pointer alignment (24 bytes on 64-bit, 12 on 32-bit).
// TestSecurityAttributesSize pins it to the real struct.
const securityAttributesSize = 3 * strconv.IntSize / 8

// Hand-fix (handfixes.json private-file-owner-only-dacl). CreatePrivateTemp
// creates a new file in dir whose DACL is set AT CREATION to one protected
// ACE: full control for the current user. Chmod is a no-op on NTFS, so a file
// created by os.CreateTemp inherits the directory's ACL (C:\Users\Public grants
// every user read) and the bytes written next are readable by other users.
// Here no inherited ACE ever exists and the file is opened with no sharing.
// pattern follows os.CreateTemp: the last "*" is replaced by a random string.
func CreatePrivateTemp(dir, pattern string) (*os.File, error) {
	sid, err := currentUserSID()
	if err != nil {
		return nil, &os.PathError{Op: "createtemp", Path: dir, Err: err}
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + sid + ")")
	if err != nil {
		return nil, &os.PathError{Op: "createtemp", Path: dir, Err: err}
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = securityAttributesSize
	prefix, suffix := pattern, ""
	if i := strings.LastIndex(pattern, "*"); i >= 0 {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}
	var last error
	for attempt := 0; attempt < 100; attempt++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		name := filepath.Join(dir, prefix+hex.EncodeToString(b[:])+suffix)
		p, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return nil, &os.PathError{Op: "createtemp", Path: name, Err: err}
		}
		h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, sa,
			windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			last = err
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "createtemp", Path: name, Err: err}
		}
		return os.NewFile(uintptr(h), name), nil
	}
	return nil, &os.PathError{Op: "createtemp", Path: dir, Err: last}
}
