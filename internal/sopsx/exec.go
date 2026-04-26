package sopsx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/williamlaffin/yalazysops/internal/secure"
)

func formatExtract(p Path) string {
	if len(p) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, seg := range p {
		switch v := seg.(type) {
		case string:
			// JSON-encode so embedded quotes/escapes round-trip into the
			// bracket form sops expects (e.g. ["weird \"key\""]).
			enc, _ := json.Marshal(v)
			b.WriteByte('[')
			b.Write(enc)
			b.WriteByte(']')
		case int:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(v))
			b.WriteByte(']')
		default:
			// Fallback: treat as JSON value to avoid silent truncation if a
			// caller passes int64/float, etc.
			enc, _ := json.Marshal(v)
			b.WriteByte('[')
			b.Write(enc)
			b.WriteByte(']')
		}
	}
	return b.String()
}

func formatDisplay(p Path) string {
	if len(p) == 0 {
		return "(root)"
	}
	var b bytes.Buffer
	for i, seg := range p {
		switch v := seg.(type) {
		case string:
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(v)
		case int:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(v))
			b.WriteByte(']')
		default:
			if i > 0 {
				b.WriteByte('.')
			}
			fmt.Fprintf(&b, "%v", v)
		}
	}
	return b.String()
}

func pathsEqual(a, b Path) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		switch av := a[i].(type) {
		case string:
			bv, ok := b[i].(string)
			if !ok || av != bv {
				return false
			}
		case int:
			bv, ok := b[i].(int)
			if !ok || av != bv {
				return false
			}
		default:
			if a[i] != b[i] {
				return false
			}
		}
	}
	return true
}

func decrypt(c *Client, file string) (*secure.Buffer, error) {
	cmd := exec.Command(c.Bin, "decrypt", file)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sops decrypt %s: %w: %s", file, err, stderr.String())
	}
	return secure.NewBuffer(stdout.Bytes()), nil
}

func extract(c *Client, file string, path Path) (*secure.Buffer, error) {
	cmd := exec.Command(c.Bin, "decrypt", "--extract", formatExtract(path), file)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sops decrypt --extract %s %s: %w: %s",
			formatDisplay(path), file, err, stderr.String())
	}
	return secure.NewBuffer(stdout.Bytes()), nil
}

func setValue(c *Client, file string, path Path, value *secure.Buffer) error {
	// JSON-encode the plaintext so sops parses it as a JSON value (matches the
	// format sops set --value-stdin expects).
	encoded, err := json.Marshal(string(value.Bytes()))
	if err != nil {
		return fmt.Errorf("sops set: encode value: %w", err)
	}
	cmd := exec.Command(c.Bin, "set", "--value-stdin", file, formatExtract(path))
	// stdin (not argv) so the plaintext never appears in /proc/<pid>/cmdline
	// or `ps` output.
	cmd.Stdin = bytes.NewReader(encoded)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sops set %s %s: %w: %s",
			formatDisplay(path), file, err, stderr.String())
	}
	return nil
}

func unsetValue(c *Client, file string, path Path) error {
	cmd := exec.Command(c.Bin, "unset", file, formatExtract(path))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sops unset %s %s: %w: %s",
			formatDisplay(path), file, err, stderr.String())
	}
	return nil
}

func isSopsFile(file string) (bool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", file, err)
	}
	// Parse as YAML — YAML is a superset of JSON, so a sops-encrypted JSON
	// file decodes through the same path. We then walk the top-level mapping
	// for a literal "sops" key, which avoids false positives from "sops:"
	// appearing inside a string value.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false, nil
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return false, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false, nil
	}
	// Mapping content is [key, value, key, value, ...].
	for i := 0; i < len(root.Content); i += 2 {
		k := root.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == "sops" {
			return true, nil
		}
	}
	return false, nil
}
