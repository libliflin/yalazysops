// Package tree builds a navigable tree of keys from a decrypted YAML or
// JSON document. Leaf nodes carry a sha256 fingerprint of their value;
// after building, plaintext is wiped immediately. The tree itself contains
// only key names and fingerprint prefixes — never plaintext values.
package tree

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
)

// Node is a position in the key tree. A node is either a branch (Children
// non-nil) or a leaf (Children nil, Fingerprint set).
type Node struct {
	// Name is the path segment at this node. For map children it's the
	// string key; for list children it's "[N]".
	Name string

	// Path is the full path from the document root to this node. Use
	// .Path.Extract() to render in sops's bracket syntax.
	Path sopsx.Path

	// Parent is nil for the root.
	Parent *Node

	// Children is nil for leaves. For maps, ordering follows source order
	// (yaml.v3 Document.Content is order-preserving). For lists, ordering
	// is index order.
	Children []*Node

	// Fingerprint is the first 8 bytes (16 hex chars) of sha256 of the
	// leaf's plaintext bytes. Empty for branches.
	Fingerprint string

	// Kind is "map", "list", "scalar", or "root".
	Kind string
}

// IsLeaf reports whether n has no children.
func (n *Node) IsLeaf() bool { return len(n.Children) == 0 && n.Kind != "map" && n.Kind != "list" }

// Walk visits n and every descendant in depth-first order.
func (n *Node) Walk(fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// FindByPath returns the node at the given path, or nil if not found.
func (n *Node) FindByPath(p sopsx.Path) *Node {
	if len(p) == 0 {
		return n
	}
	for _, c := range n.Children {
		if len(c.Path) > 0 && pathSegEqual(c.Path[len(c.Path)-1], p[0]) {
			return c.FindByPath(p[1:])
		}
	}
	return nil
}

// Build parses doc as YAML (which is a superset of JSON for this purpose)
// and returns the root of the key tree. The sops metadata block (top-level
// "sops" key) is excluded. doc is not modified; callers are still
// responsible for wiping the buffer that owned doc.
func Build(doc []byte) (*Node, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, fmt.Errorf("parse decrypted document: %w", err)
	}
	if root.Kind == 0 {
		return &Node{Kind: "root"}, nil
	}
	// yaml.Unmarshal wraps everything in a DocumentNode whose first content
	// is the actual root.
	var top *yaml.Node
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return &Node{Kind: "root"}, nil
		}
		top = root.Content[0]
	} else {
		top = &root
	}
	out := &Node{Kind: "root"}
	walk(top, sopsx.Path{}, out)
	return out, nil
}

func walk(n *yaml.Node, path sopsx.Path, parent *Node) {
	switch n.Kind {
	case yaml.MappingNode:
		// Pairs come as alternating key/value entries.
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			// Skip sops metadata at the document root.
			if len(path) == 0 && k == "sops" {
				continue
			}
			v := n.Content[i+1]
			child := &Node{
				Name:   k,
				Path:   appendPath(path, k),
				Parent: parent,
			}
			parent.Children = append(parent.Children, child)
			switch v.Kind {
			case yaml.MappingNode:
				child.Kind = "map"
				walk(v, child.Path, child)
			case yaml.SequenceNode:
				child.Kind = "list"
				walk(v, child.Path, child)
			default:
				child.Kind = "scalar"
				child.Fingerprint = fingerprint(v.Value)
			}
		}
	case yaml.SequenceNode:
		for i, v := range n.Content {
			child := &Node{
				Name:   "[" + strconv.Itoa(i) + "]",
				Path:   appendPath(path, i),
				Parent: parent,
			}
			parent.Children = append(parent.Children, child)
			switch v.Kind {
			case yaml.MappingNode:
				child.Kind = "map"
				walk(v, child.Path, child)
			case yaml.SequenceNode:
				child.Kind = "list"
				walk(v, child.Path, child)
			default:
				child.Kind = "scalar"
				child.Fingerprint = fingerprint(v.Value)
			}
		}
	}
}

func appendPath(p sopsx.Path, seg any) sopsx.Path {
	out := make(sopsx.Path, len(p)+1)
	copy(out, p)
	out[len(p)] = seg
	return out
}

// fingerprint returns the first 16 hex chars of sha256(value). The value is
// hashed as a string but never retained; the hash is one-way so the prefix
// is safe to display.
func fingerprint(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:8])
}

// FingerprintBytes hashes raw bytes (e.g. a leaf value pulled directly into
// a secure buffer). The buffer's bytes are read but not modified.
func FingerprintBytes(b *secure.Buffer) string {
	h := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(h[:8])
}

func pathSegEqual(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	}
	return false
}

// SortedKeys returns the leaf and branch names directly under n in source
// order. Currently a stub for future "sort alphabetically" toggle.
func (n *Node) SortedKeys() []string {
	keys := make([]string, len(n.Children))
	for i, c := range n.Children {
		keys[i] = c.Name
	}
	sort.Strings(keys)
	return keys
}
