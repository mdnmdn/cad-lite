package appenv

import "testing"

func TestLookupPrefersNewName(t *testing.T) {
	t.Setenv("CADLITE_APPENV_X", "new")
	t.Setenv("GODRAW_APPENV_X", "old")
	if v, ok := Lookup("APPENV_X"); !ok || v != "new" {
		t.Fatalf("Lookup = %q, %v; want new, true", v, ok)
	}
}

func TestLookupFallsBackToLegacyName(t *testing.T) {
	t.Setenv("GODRAW_APPENV_Y", "old")
	if v := Get("APPENV_Y"); v != "old" {
		t.Fatalf("Get = %q, want old", v)
	}
}

func TestEmptyNewNameShadowsLegacy(t *testing.T) {
	t.Setenv("CADLITE_APPENV_Z", "")
	t.Setenv("GODRAW_APPENV_Z", "old")
	if v, ok := Lookup("APPENV_Z"); !ok || v != "" {
		t.Fatalf("Lookup = %q, %v; want empty, true", v, ok)
	}
}

func TestUnset(t *testing.T) {
	if v, ok := Lookup("APPENV_UNSET_FOR_SURE"); ok || v != "" {
		t.Fatalf("Lookup = %q, %v; want empty, false", v, ok)
	}
}
