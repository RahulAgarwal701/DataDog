package registry

import "testing"

func TestLoadTestdata(t *testing.T) {
	r, err := Load("../../testdata/registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := r.ByPort(8002); !ok || s.Name != "payments" {
		t.Fatalf("ByPort(8002) = %+v, %v", s, ok)
	}
	if s, ok := r.ByName("loadgen"); !ok || s.Kind != "external" {
		t.Fatalf("loadgen = %+v, %v", s, ok)
	}
	if r.KindOf("prometheus") != "external" {
		t.Fatal("unknown names must be external")
	}
	if len(r.Services()) != 5 {
		t.Fatalf("want 5 services, got %d", len(r.Services()))
	}
}

func TestParseRejectsDuplicates(t *testing.T) {
	_, err := Parse([]byte("services:\n  - {name: a, port: 1}\n  - {name: b, port: 1}\n"))
	if err == nil {
		t.Fatal("duplicate port accepted")
	}
	_, err = Parse([]byte("services:\n  - {name: a, port: 1}\n  - {name: a, port: 2}\n"))
	if err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err = Parse([]byte("services: []\n")); err == nil {
		t.Fatal("empty registry accepted")
	}
}
