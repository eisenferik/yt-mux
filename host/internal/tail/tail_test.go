package tail

import "testing"

func TestBufferIsBoundedAndRedactsPaths(t *testing.T) {
	buffer := New(2, `C:\secret\cookies.txt`)
	buffer.Add("first")
	buffer.Add(`using C:\secret\cookies.txt`)
	buffer.Add("last")
	lines := buffer.Lines()
	if len(lines) != 2 || lines[0] != "using [temporary cookie file]" || lines[1] != "last" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}
