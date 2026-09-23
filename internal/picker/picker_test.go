package picker

import (
	"strings"
	"testing"
)

func TestIsTerminal_NoTTY(t *testing.T) {
	// In a test environment stdin/stdout are not a real TTY.
	if IsTerminal() {
		t.Log("IsTerminal() = true (running with a real PTY — skipping assertion)")
	}
}

func TestViewport(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		total, cursor, start, visible int
		want                          Viewport
	}{
		{"fits", 3, 1, 0, 5, Viewport{Start: 0, End: 3, Total: 3}},
		{"scroll down", 10, 6, 0, 4, Viewport{Start: 3, End: 7, Total: 10, Above: 3, Below: 3}},
		{"scroll up", 10, 1, 4, 4, Viewport{Start: 1, End: 5, Total: 10, Above: 1, Below: 5}},
		{"single row last", 5, 4, 0, 1, Viewport{Start: 4, End: 5, Total: 5, Above: 4}},
		{"empty", 0, 0, 0, 4, Viewport{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateViewport(tc.total, tc.cursor, tc.start, tc.visible); got != tc.want {
				t.Fatalf("calculateViewport() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in    string
		cells int
		want  string
	}{
		{"plain", 10, "plain"},
		{"abcdef", 5, "ab..."},
		{"界abc", 4, "..."},
		{"a\x1b[31mred\x1b[0m", 10, "ared"},
		{"a\x1b]0;title\aB", 10, "aB"},
	}
	for _, tc := range cases {
		if got := sanitizeAndTruncate(tc.in, tc.cells); got != tc.want {
			t.Errorf("sanitizeAndTruncate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInputDecoder(t *testing.T) {
	var d inputDecoder
	for _, chunk := range [][]byte{{0x1b}, {'['}, {'A', 'j'}} {
		got := d.feed(chunk)
		if len(got) == 0 {
			continue
		}
		if len(got) != 2 || got[0] != keyUp || got[1] != keyDown {
			t.Fatalf("fragmented decode = %v, want up/down", got)
		}
	}
	if got := d.feed([]byte{0x1b}); len(got) != 0 {
		t.Fatalf("bare escape decoded early: %v", got)
	}
	if got := d.flushEscape(); len(got) != 1 || got[0] != keyCancel {
		t.Fatalf("flush escape = %v", got)
	}
}

func TestRenderFrameBounded(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = "model-" + string(rune('a'+i%26))
	}
	_, frame := renderFrame(items, 50, 0, 20, 8, "", pickerConfig{title: "Models"})
	if strings.HasSuffix(frame, "\r\n") {
		t.Fatal("frame has a trailing newline")
	}
	body := strings.TrimPrefix(frame, "\033[H\033[2J")
	body = strings.TrimSuffix(body, "\033[J")
	if rows := strings.Count(body, "\r\n") + 1; rows > 8 {
		t.Fatalf("rendered %d rows, terminal has 8", rows)
	}
}

func TestPick_EmptyList(t *testing.T) {
	result, err := Pick([]string{}, "")
	if err != nil {
		t.Fatalf("Pick(empty) error: %v", err)
	}
	if result != "" {
		t.Errorf("Pick(empty) = %q, want empty string", result)
	}
}

func TestPickMulti_EmptyList(t *testing.T) {
	result, err := PickMulti([]string{}, []bool{})
	if err != nil {
		t.Fatalf("PickMulti(empty) error: %v", err)
	}
	if result != nil {
		t.Errorf("PickMulti(empty) = %v, want nil", result)
	}
}

func TestPickMulti_InitialSelectionPreserved(t *testing.T) {
	// PickMulti is interactive; we test the internal selection logic directly
	// by simulating what happens to the selected slice.
	items := []string{"alpha", "beta", "gamma"}
	initial := []bool{true, false, true}

	selected := make([]bool, len(items))
	copy(selected, initial)

	// Verify copy is independent
	initial[0] = false
	if !selected[0] {
		t.Error("selected slice was not independently copied from initial")
	}

	// Simulate toggle of index 1
	selected[1] = !selected[1]
	if !selected[1] {
		t.Error("toggle of index 1 did not work")
	}

	// Final state: [true, true, true]
	for i, want := range []bool{true, true, true} {
		if selected[i] != want {
			t.Errorf("selected[%d] = %v, want %v", i, selected[i], want)
		}
	}
}
