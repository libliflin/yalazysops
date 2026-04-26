package tree

import (
	"testing"
)

func TestBuild_FlatMap(t *testing.T) {
	doc := []byte(`
anthropic_api_key: sk-ant-test123
postgres_password: hunter2
`)
	root, err := Build(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 2 {
		t.Fatalf("want 2 children, got %d", len(root.Children))
	}
	if root.Children[0].Name != "anthropic_api_key" {
		t.Errorf("first child name = %q", root.Children[0].Name)
	}
	if root.Children[0].Fingerprint == "" {
		t.Error("scalar leaf missing fingerprint")
	}
	if len(root.Children[0].Fingerprint) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(root.Children[0].Fingerprint))
	}
}

func TestBuild_Nested(t *testing.T) {
	doc := []byte(`
db:
  prod:
    password: hunter2
    host: db.example.com
  dev:
    password: dev123
`)
	root, err := Build(doc)
	if err != nil {
		t.Fatal(err)
	}
	db := root.Children[0]
	if db.Name != "db" || db.Kind != "map" {
		t.Fatalf("db node = %+v", db)
	}
	if len(db.Children) != 2 {
		t.Fatalf("db has %d children, want 2", len(db.Children))
	}
	prod := db.Children[0]
	if prod.Name != "prod" {
		t.Errorf("prod.Name = %q", prod.Name)
	}
	if len(prod.Children) != 2 {
		t.Errorf("prod has %d children", len(prod.Children))
	}
	pw := prod.Children[0]
	if pw.Name != "password" || pw.Fingerprint == "" {
		t.Errorf("password leaf = %+v", pw)
	}
	// Path should be ["db", "prod", "password"]
	if len(pw.Path) != 3 {
		t.Fatalf("password path len = %d", len(pw.Path))
	}
	if pw.Path[0] != "db" || pw.Path[1] != "prod" || pw.Path[2] != "password" {
		t.Errorf("password path = %v", pw.Path)
	}
}

func TestBuild_SkipsSopsMetadata(t *testing.T) {
	doc := []byte(`
api_key: foo
sops:
  kms: []
  age:
    - recipient: age1xxx
`)
	root, err := Build(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range root.Children {
		if c.Name == "sops" {
			t.Fatalf("sops metadata leaked into tree")
		}
	}
}

func TestBuild_List(t *testing.T) {
	doc := []byte(`
trusted_origins:
  - https://a.com
  - https://b.com
`)
	root, err := Build(doc)
	if err != nil {
		t.Fatal(err)
	}
	list := root.Children[0]
	if list.Kind != "list" {
		t.Errorf("kind = %q", list.Kind)
	}
	if len(list.Children) != 2 {
		t.Fatalf("list has %d children", len(list.Children))
	}
	if list.Children[0].Name != "[0]" {
		t.Errorf("first child name = %q", list.Children[0].Name)
	}
	if list.Children[0].Path[1] != 0 {
		t.Errorf("path index = %v", list.Children[0].Path[1])
	}
}

func TestFingerprint_Stable(t *testing.T) {
	a := fingerprint("hunter2")
	b := fingerprint("hunter2")
	if a != b {
		t.Errorf("fingerprint not stable: %q vs %q", a, b)
	}
	c := fingerprint("hunter3")
	if a == c {
		t.Errorf("fingerprint collision on different inputs")
	}
}
