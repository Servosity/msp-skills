//go:build windows

// PROOF ONLY (DO NOT MERGE): exercises the fixed guard against real Windows
// security descriptors on a runner whose account is the built-in Administrator.
package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWinAdminProofResolver(t *testing.T) {
	me, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("current user SID: %s (RID 500: %v)", me, strings.HasSuffix(me, "-500"))
	for _, a := range []string{"LA", "LG", "BA", "SY", "WD", "BU", "DA", "ZZ"} {
		sid, ok := resolveSDDLAlias(a)
		t.Logf("resolveSDDLAlias(%s) = %q ok=%v", a, sid, ok)
	}
	if !strings.HasSuffix(me, "-500") {
		t.Fatalf("runner account is not RID 500; this proof needs the built-in Administrator")
	}
	if la, ok := resolveSDDLAlias("LA"); !ok || la != me {
		t.Fatalf("LA resolved to %q ok=%v, want %s", la, ok, me)
	}
	if _, ok := resolveSDDLAlias("ZZ"); ok {
		t.Fatalf("unknown alias ZZ resolved")
	}
}

func TestWinAdminProofRealDescriptors(t *testing.T) {
	me, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	other := os.Getenv("WINPROOF_OTHER_SID")
	if other == "" {
		t.Fatal("WINPROOF_OTHER_SID not set (the workflow creates a second local account)")
	}
	dir := t.TempDir()
	cases := []struct {
		name    string
		sddl    string
		wantErr string
	}{
		{"owner me, only my ACE", "O:" + me + "D:PAI(A;;FA;;;" + me + ")", ""},
		{"owner me, me+SYSTEM+Admins", "O:" + me + "D:PAI(A;;FA;;;" + me + ")(A;;FA;;;SY)(A;;FA;;;BA)", ""},
		{"owner Admins (elevated default), me+SYSTEM+Admins", "O:BAD:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + me + ")", ""},
		{"plus BUILTIN Users", "O:" + me + "D:PAI(A;;FA;;;" + me + ")(A;;FR;;;BU)", "broad group"},
		{"plus Everyone", "O:" + me + "D:PAI(A;;FA;;;" + me + ")(A;;FR;;;WD)", "broad group"},
		{"plus Authenticated Users", "O:" + me + "D:PAI(A;;FA;;;" + me + ")(A;;FR;;;AU)", "broad group"},
		{"plus a second local user", "O:" + me + "D:PAI(A;;FA;;;" + me + ")(A;;FR;;;" + other + ")", "non-owner principal"},
		{"plus local Guest (LG)", "O:" + me + "D:PAI(A;;FA;;;" + me + ")(A;;FR;;;S-1-5-21-" + strings.Join(strings.Split(me, "-")[4:7], "-") + "-501)", "non-owner principal"},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(dir, "f"+string(rune('a'+i)))
			if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			sd, err := windows.SecurityDescriptorFromString(c.sddl)
			if err != nil {
				t.Fatalf("SDDL %q: %v", c.sddl, err)
			}
			owner, _, err := sd.Owner()
			if err != nil {
				t.Fatal(err)
			}
			dacl, _, err := sd.DACL()
			if err != nil {
				t.Fatal(err)
			}
			if err := windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT,
				windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
				owner, nil, dacl, nil); err != nil {
				t.Fatalf("SetNamedSecurityInfo: %v", err)
			}
			got, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT,
				windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			verr := VerifyCredsPerms(p)
			t.Logf("on-disk SDDL: %s -> VerifyCredsPerms: %v", got.String(), verr)
			if c.wantErr == "" {
				if verr != nil {
					t.Fatalf("refused: %v", verr)
				}
				return
			}
			if verr == nil || !strings.Contains(verr.Error(), c.wantErr) {
				t.Fatalf("err=%v, want a refusal containing %q", verr, c.wantErr)
			}
		})
	}
}

// Informational: how a NULL DACL renders on disk, with and without the
// protected flag, and whether the guard refuses each.
func TestWinAdminProofNullDACL(t *testing.T) {
	me, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for i, protect := range []bool{false, true} {
		p := filepath.Join(dir, "null"+string(rune('a'+i)))
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		sd, err := windows.SecurityDescriptorFromString("O:" + me)
		if err != nil {
			t.Fatal(err)
		}
		owner, _, _ := sd.Owner()
		info := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.OWNER_SECURITY_INFORMATION)
		if protect {
			info |= windows.PROTECTED_DACL_SECURITY_INFORMATION
		}
		if err := windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, info, owner, nil, nil, nil); err != nil {
			t.Fatalf("SetNamedSecurityInfo(protect=%v): %v", protect, err)
		}
		got, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		verr := VerifyCredsPerms(p)
		t.Logf("protect=%v on-disk SDDL: %s -> VerifyCredsPerms: %v", protect, got.String(), verr)
		if verr == nil {
			t.Errorf("protect=%v: NULL DACL accepted", protect)
		}
	}
}
