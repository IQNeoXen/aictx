package picker

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// IsTerminal returns true if stdin/stdout are terminals.
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stdin.Fd())
}

type Viewport struct {
	Start int
	End   int
	Total int
	Above int
	Below int
}

func calculateViewport(totalItems, cursor, windowStart, visibleCount int) Viewport {
	if totalItems <= 0 || visibleCount <= 0 {
		return Viewport{Total: max(totalItems, 0)}
	}
	cursor = min(max(cursor, 0), totalItems-1)
	visibleCount = min(visibleCount, totalItems)
	windowStart = min(max(windowStart, 0), totalItems-visibleCount)
	if cursor < windowStart {
		windowStart = cursor
	} else if cursor >= windowStart+visibleCount {
		windowStart = cursor - visibleCount + 1
	}
	end := windowStart + visibleCount
	return Viewport{Start: windowStart, End: end, Total: totalItems, Above: windowStart, Below: totalItems - end}
}

// sanitizeAndTruncate removes terminal controls and limits text by display cells.
func sanitizeAndTruncate(s string, maxCells int) string {
	var clean strings.Builder
	state := byte(0)
	for _, r := range s {
		if state != 0 {
			switch state {
			case 1:
				if r == '[' {
					state = 2
				} else if r == ']' {
					state = 3
				} else {
					state = 0
				}
			case 2:
				if r >= 0x40 && r <= 0x7e {
					state = 0
				}
			case 3:
				if r == '\a' {
					state = 0
				} else if r == '\x1b' {
					state = 4
				}
			case 4:
				if r == '\\' {
					state = 0
				} else {
					state = 3
				}
			}
			continue
		}
		if r == '\x1b' {
			state = 1
			continue
		}
		if r == '\u200d' || unicode.IsControl(r) {
			continue
		}
		clean.WriteRune(r)
	}
	text := clean.String()
	if maxCells <= 0 {
		return ""
	}
	if cellWidth(text) <= maxCells {
		return text
	}
	if maxCells < 3 {
		return strings.Repeat(".", maxCells)
	}
	limit := maxCells - 3
	var out strings.Builder
	width := 0
	for _, r := range text {
		w := runeWidth(r)
		if width+w > limit {
			break
		}
		out.WriteRune(r)
		width += w
	}
	return out.String() + "..."
}

func cellWidth(s string) int {
	width := 0
	for _, r := range s {
		width += runeWidth(r)
	}
	return width
}

func runeWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '\u200d' {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a || (r >= 0x2e80 && r <= 0xa4cf) || (r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0xfe10 && r <= 0xfe19) || (r >= 0xfe30 && r <= 0xfe6f) || (r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6)) {
		return 2
	}
	return 1
}

type inputKey int

const (
	keyNone inputKey = iota
	keyUp
	keyDown
	keyEnter
	keyCancel
	keyToggle
)

// inputDecoder retains incomplete escape sequences between reads.
type inputDecoder struct{ pending []byte }

func (d *inputDecoder) feed(data []byte) []inputKey {
	d.pending = append(d.pending, data...)
	var keys []inputKey
	for len(d.pending) > 0 {
		b := d.pending[0]
		if b == 0x1b {
			if len(d.pending) == 1 {
				break
			}
			if d.pending[1] != '[' {
				keys = append(keys, keyCancel)
				d.pending = d.pending[1:]
				continue
			}
			if len(d.pending) < 3 {
				break
			}
			switch d.pending[2] {
			case 'A':
				keys = append(keys, keyUp)
			case 'B':
				keys = append(keys, keyDown)
			}
			d.pending = d.pending[3:]
			continue
		}
		switch b {
		case '\r', '\n':
			keys = append(keys, keyEnter)
		case 3, 4, 'q':
			keys = append(keys, keyCancel)
		case 'k':
			keys = append(keys, keyUp)
		case 'j':
			keys = append(keys, keyDown)
		case ' ':
			keys = append(keys, keyToggle)
		}
		d.pending = d.pending[1:]
	}
	return keys
}

func (d *inputDecoder) flushEscape() []inputKey {
	if len(d.pending) > 0 && d.pending[0] == 0x1b {
		d.pending = nil
		return []inputKey{keyCancel}
	}
	return nil
}

type pickerConfig struct {
	title      string
	subtitle   string
	banner     bool
	maxVisible int
}

type Option func(*pickerConfig)

func WithTitle(title string) Option       { return func(c *pickerConfig) { c.title = title } }
func WithSubtitle(subtitle string) Option { return func(c *pickerConfig) { c.subtitle = subtitle } }
func WithBanner(enabled bool) Option      { return func(c *pickerConfig) { c.banner = enabled } }
func WithMaxVisible(max int) Option       { return func(c *pickerConfig) { c.maxVisible = max } }

// Pick shows a bounded, terminal-adaptive list picker. Empty selection means cancelled.
func Pick(items []string, current string, opts ...Option) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	cfg := pickerConfig{title: fmt.Sprintf("Select an item (%d available)", len(items)), banner: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	cursor := 0
	for i, item := range items {
		if item == current {
			cursor = i
			break
		}
	}

	if !IsTerminal() || os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return plainPick(items, current)
	}
	width, height, sizeErr := term.GetSize(int(os.Stdout.Fd()))
	if sizeErr != nil || height < 8 {
		return plainPick(items, current)
	}
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("enabling raw mode: %w", err)
	}
	if err := prepareInput(os.Stdin); err != nil {
		_ = term.Restore(fd, oldState)
		return "", fmt.Errorf("configuring picker input: %w", err)
	}

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			fmt.Fprint(os.Stdout, "\033[?1049l\033[?25h")
			restoreInput(os.Stdin)
			_ = term.Restore(fd, oldState)
		})
	}
	defer cleanup()
	fmt.Fprint(os.Stdout, "\033[?1049h\033[H\033[2J\033[?25l")

	inputPoll := time.NewTicker(10 * time.Millisecond)
	defer inputPoll.Stop()
	buf := make([]byte, 64)
	resize := make(chan os.Signal, 1)
	terminated := make(chan os.Signal, 1)
	notifyResize(resize)
	notifyTerminate(terminated)
	defer signal.Stop(resize)
	defer signal.Stop(terminated)

	decoder := &inputDecoder{}
	windowStart := 0
	visible, frame := renderFrame(items, cursor, windowStart, width, height, current, cfg)
	windowStart = visible.Start
	fmt.Fprint(os.Stdout, frame)
	var escTimer *time.Timer
	var escC <-chan time.Time
	for {
		select {
		case <-inputPoll.C:
			n, readErr := readInput(os.Stdin, buf)
			if readErr != nil && !isWouldBlock(readErr) {
				return "", fmt.Errorf("reading picker input: %w", readErr)
			}
			if n == 0 {
				continue
			}
			var keys []inputKey
			if n > 0 {
				keys = decoder.feed(buf[:n])
			}
			if len(decoder.pending) > 0 && decoder.pending[0] == 0x1b {
				if escTimer == nil {
					escTimer = time.NewTimer(25 * time.Millisecond)
					escC = escTimer.C
				}
			} else if escTimer != nil {
				if !escTimer.Stop() {
					select {
					case <-escTimer.C:
					default:
					}
				}
				escTimer, escC = nil, nil
			}
			for _, key := range keys {
				if key == keyEnter {
					return items[cursor], nil
				}
				if key == keyCancel {
					return "", nil
				}
				if key == keyUp {
					cursor--
				}
				if key == keyDown {
					cursor++
				}
				cursor = min(max(cursor, 0), len(items)-1)
			}
			width, height, _ = term.GetSize(int(os.Stdout.Fd()))
			visible, frame = renderFrame(items, cursor, windowStart, width, height, current, cfg)
			windowStart = visible.Start
			fmt.Fprint(os.Stdout, frame)
		case <-escC:
			for _, key := range decoder.flushEscape() {
				if key == keyCancel {
					return "", nil
				}
			}
			escTimer, escC = nil, nil
			width, height, _ = term.GetSize(int(os.Stdout.Fd()))
			visible, frame = renderFrame(items, cursor, windowStart, width, height, current, cfg)
			windowStart = visible.Start
			fmt.Fprint(os.Stdout, frame)
		case <-resize:
			width, height, sizeErr = term.GetSize(int(os.Stdout.Fd()))
			if sizeErr == nil {
				visible, frame = renderFrame(items, cursor, windowStart, width, height, current, cfg)
				windowStart = visible.Start
				fmt.Fprint(os.Stdout, frame)
			}
		case sig := <-terminated:
			cleanup()
			if sig == os.Interrupt {
				return "", nil
			}
			return "", fmt.Errorf("terminated by %s", sig)
		}
	}
}

func plainPick(items []string, current string) (string, error) {
	fmt.Printf("Available items: %s\nSelect [%s]: ", strings.Join(items, ", "), current)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return current, nil
}

func renderFrame(items []string, cursor, windowStart, width, height int, current string, cfg pickerConfig) (Viewport, string) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	lines := make([]string, 0, height)
	if cfg.banner && height >= 14 {
		lines = append(lines, "▄▀█ █ █▀▀ ▀█▀ ▀▄▀   aictx • AI Context Switcher", "█▀█ █ █▄▄  █  █ █   "+cfg.subtitle, strings.Repeat("─", max(width, 1)))
	}
	if cfg.title != "" {
		lines = append(lines, cfg.title)
	}
	if cfg.subtitle != "" && (!cfg.banner || height < 14) {
		lines = append(lines, cfg.subtitle)
	}
	lines = append(lines, "")
	if len(lines)+3 > height { // top indicator, bottom indicator, and footer
		lines = lines[:max(0, height-3)]
	}
	itemRows := height - len(lines) - 3
	if cfg.maxVisible > 0 {
		itemRows = min(itemRows, cfg.maxVisible)
	}
	if itemRows < 1 {
		return Viewport{Total: len(items)}, "\033[H\033[2J"
	}
	vp := calculateViewport(len(items), cursor, windowStart, itemRows)
	lines = append(lines, indicator(vp.Above, true))
	for i := vp.Start; i < vp.End; i++ {
		marker := "  "
		if items[i] == current {
			marker = "* "
		}
		lines = append(lines, marker+sanitizeAndTruncate(items[i], max(0, width-2)))
	}
	lines = append(lines, indicator(vp.Below, false), "↑/↓ / j/k move • Enter select • Esc / q cancel")
	if len(lines) > height {
		lines = lines[:height]
	}
	firstItemRow := len(lines) - (vp.End - vp.Start) - 2
	for i := range lines {
		lines[i] = sanitizeAndTruncate(lines[i], width)
	}
	selectedRow := firstItemRow + cursor - vp.Start
	if selectedRow >= 0 && selectedRow < len(lines) {
		marker := "  "
		if items[cursor] == current {
			marker = "* "
		}
		lines[selectedRow] = "\033[1;7m" + marker + sanitizeAndTruncate(items[cursor], max(0, width-2)) + "\033[0m"
	}
	return vp, "\033[H\033[2J" + strings.Join(lines, "\r\n") + "\033[J"
}

func indicator(count int, above bool) string {
	if count <= 0 {
		return ""
	}
	if above {
		return fmt.Sprintf("↑ %d more above", count)
	}
	return fmt.Sprintf("↓ %d more below", count)
}

func stripANSI(s string) string { return sanitizeAndTruncate(s, int(^uint(0)>>1)) }

// PickMulti shows an interactive checkbox picker (legacy, not viewport bounded).
// items and initialSelected must have the same length.
func PickMulti(items []string, initialSelected []bool) ([]bool, error) {
	if len(items) == 0 {
		return nil, nil
	}
	selected := make([]bool, len(items))
	copy(selected, initialSelected)
	cursor := 0
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("enabling raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")
	renderMulti(items, selected, cursor)
	fmt.Print("\r\n  \033[2m↑/↓ move   Space toggle   Enter confirm   Esc cancel\033[0m\r\n")
	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil, err
		}
		switch {
		case n == 1 && (buf[0] == 13 || buf[0] == 10):
			clearList(len(items) + 2)
			return selected, nil
		case n == 1 && (buf[0] == 3 || buf[0] == 27 || buf[0] == 'q'):
			clearList(len(items) + 2)
			return nil, nil
		case n == 1 && buf[0] == ' ':
			selected[cursor] = !selected[cursor]
		case n == 1 && buf[0] == 'k':
			if cursor > 0 {
				cursor--
			}
		case n == 1 && buf[0] == 'j':
			if cursor < len(items)-1 {
				cursor++
			}
		case n == 3 && buf[0] == 27 && buf[1] == '[':
			if buf[2] == 'A' && cursor > 0 {
				cursor--
			}
			if buf[2] == 'B' && cursor < len(items)-1 {
				cursor++
			}
		default:
			continue
		}
		fmt.Printf("\033[%dA", len(items)+2)
		renderMulti(items, selected, cursor)
		fmt.Print("\r\n  \033[2m↑/↓ move   Space toggle   Enter confirm   Esc cancel\033[0m\r\n")
	}
}

func renderMulti(items []string, selected []bool, cursor int) {
	for i, item := range items {
		check := "[ ]"
		if i < len(selected) && selected[i] {
			check = "[x]"
		}
		line := fmt.Sprintf("  %s %s", check, item)
		if i == cursor {
			fmt.Printf("\033[1;7m%s\033[0m\033[K\r\n", line)
		} else {
			fmt.Printf("%s\033[K\r\n", line)
		}
	}
}

func render(items []string, cursor int, current string) { // retained for package-local compatibility
	renderMulti(items, make([]bool, len(items)), cursor)
	_ = current
}

func clearList(n int) {
	fmt.Printf("\033[%dA", n)
	for i := 0; i < n; i++ {
		fmt.Print("\033[K\r\n")
	}
	fmt.Printf("\033[%dA", n)
}
