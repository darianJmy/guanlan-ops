package shellutil

import (
	"os/exec"
	"testing"
)

func TestCommandPreservesLiteralArguments(t *testing.T) {
	for _, arg := range []string{"", "中文 text", "a'b\"c", "$(printf injected); echo bad", "line\nnext", "\\$HOME"} {
		out, err := exec.Command("sh", "-c", Command("printf", []string{"%s", arg})).Output()
		if err != nil || string(out) != arg {
			t.Fatalf("argument %q became %q: %v", arg, out, err)
		}
	}
}
func TestIdentifierAndLineEndings(t *testing.T) {
	for _, value := range []string{"NAME", "_A1", "a_2"} {
		if !IsIdentifier(value) {
			t.Errorf("valid identifier rejected: %q", value)
		}
	}
	for _, value := range []string{"", "1A", "A-B", "A;id", "A\nB"} {
		if IsIdentifier(value) {
			t.Errorf("invalid identifier accepted: %q", value)
		}
	}
	if got := NormalizeLineEndings("a\r\nb\rc\n"); got != "a\nb\nc\n" {
		t.Fatalf("line endings: %q", got)
	}
}
