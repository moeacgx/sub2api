package service

import "testing"

func TestParseVersion_SupportsFourthSegment(t *testing.T) {
	got := parseVersion("v0.1.126.1")
	want := [4]int{0, 1, 126, 1}
	if got != want {
		t.Fatalf("parseVersion() = %#v, want %#v", got, want)
	}
}

func TestCompareVersions_UsesFourthSegment(t *testing.T) {
	if got := compareVersions("0.1.126", "0.1.126.1"); got >= 0 {
		t.Fatalf("compareVersions() = %d, want < 0", got)
	}
	if got := compareVersions("0.1.126.2", "0.1.126.1"); got <= 0 {
		t.Fatalf("compareVersions() = %d, want > 0", got)
	}
}
