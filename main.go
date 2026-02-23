package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/term"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

var tags = []string{"TODO", "FIXME", "HACK", "NOTE"}

var tagColors = map[string]string{
	"TODO":  "\033[33m", // yellow
	"FIXME": "\033[31m", // red
	"HACK":  "\033[35m", // magenta
	"NOTE":  "\033[36m", // cyan
}

var ignoreDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true,
	".venv": true, "venv": true, "dist": true, "build": true,
	".next": true, ".cache": true, "target": true, ".idea": true,
	".vscode": true,
}

var ignoreExts = map[string]bool{
	".pyc": true, ".pyo": true, ".so": true, ".o": true, ".a": true, ".dylib": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".ico": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true,
	".pdf": true, ".doc": true, ".docx": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".exe": true, ".dll": true,
}

// ANSI escape helpers
const (
	reset     = "\033[0m"
	bold      = "\033[1m"
	dim       = "\033[2m"
	reverse   = "\033[7m"
	hideCur   = "\033[?25l"
	showCur   = "\033[?25h"
	clearScr  = "\033[2J"
	clearLine = "\033[2K"
	green     = "\033[32m"
	white     = "\033[37m"
)

func moveTo(row, col int) string {
	return fmt.Sprintf("\033[%d;%dH", row, col)
}

// ---------------------------------------------------------------------------
// Data
// ---------------------------------------------------------------------------

type Annotation struct {
	Tag        string
	Text       string
	FilePath   string
	LineNumber int
	RelPath    string
}

// ---------------------------------------------------------------------------
// Scanner
// ---------------------------------------------------------------------------

func scanDirectory(root string) []Annotation {
	pattern := regexp.MustCompile(
		`(?:#|//|/\*|<!--|--)\s*(` + strings.Join(tags, "|") + `)[\s:：]*(.*?)(?:\s*\*/|\s*-->)?$`,
	)

	var results []Annotation
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			if ignoreDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ignoreExts[ext] {
			return nil
		}

		// Skip files larger than 1MB
		if info.Size() > 1024*1024 {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			matches := pattern.FindStringSubmatch(line)
			if matches != nil {
				tag := strings.ToUpper(matches[1])
				text := strings.TrimSpace(matches[2])
				results = append(results, Annotation{
					Tag:        tag,
					Text:       text,
					FilePath:   path,
					LineNumber: lineNo,
					RelPath:    rel,
				})
			}
		}
		return nil
	})

	sort.Slice(results, func(i, j int) bool {
		if results[i].RelPath != results[j].RelPath {
			return results[i].RelPath < results[j].RelPath
		}
		return results[i].LineNumber < results[j].LineNumber
	})

	return results
}

// ---------------------------------------------------------------------------
// Terminal raw mode (cross-platform via golang.org/x/term)
// ---------------------------------------------------------------------------

type termState struct {
	old *term.State
}

func enableRawMode() (*termState, error) {
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	return &termState{old: old}, nil
}

func (t *termState) restore() {
	term.Restore(int(os.Stdin.Fd()), t.old)
}

func getTermSize() (width, height int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80, 24
	}
	return w, h
}

// readKey reads a single key press, handling escape sequences for arrow keys.
func readKey() string {
	buf := make([]byte, 8)
	n, err := os.Stdin.Read(buf)
	if err != nil || n == 0 {
		return ""
	}

	// Escape sequence
	if buf[0] == 0x1b && n >= 3 {
		if buf[1] == '[' {
			switch buf[2] {
			case 'A':
				return "up"
			case 'B':
				return "down"
			case 'C':
				return "right"
			case 'D':
				return "left"
			}
		}
		return "esc"
	}

	switch buf[0] {
	case 13, 10:
		return "enter"
	case 9:
		return "tab"
	case 127, 8:
		return "backspace"
	case 0x1b:
		return "esc"
	default:
		return string(buf[:n])
	}
}

// ---------------------------------------------------------------------------
// TUI App
// ---------------------------------------------------------------------------

type App struct {
	all      []Annotation
	filtered []Annotation
	root     string

	filterTag string // "" = all
	sortByTag bool   // false = by file, true = by tag type
	cursor    int
	scroll    int
	message   string

	cmdMode bool
	cmdBuf  string
}

func newApp(annotations []Annotation, root string) *App {
	a := &App{
		all:       annotations,
		filtered:  append([]Annotation{}, annotations...),
		root:      root,
		sortByTag: true,
	}
	a.sortFiltered()
	return a
}

func (a *App) sortFiltered() {
	if a.sortByTag {
		tagOrder := make(map[string]int, len(tags))
		for i, t := range tags {
			tagOrder[t] = i
		}
		sort.SliceStable(a.filtered, func(i, j int) bool {
			oi, oj := tagOrder[a.filtered[i].Tag], tagOrder[a.filtered[j].Tag]
			if oi != oj {
				return oi < oj
			}
			if a.filtered[i].RelPath != a.filtered[j].RelPath {
				return a.filtered[i].RelPath < a.filtered[j].RelPath
			}
			return a.filtered[i].LineNumber < a.filtered[j].LineNumber
		})
	} else {
		sort.SliceStable(a.filtered, func(i, j int) bool {
			if a.filtered[i].RelPath != a.filtered[j].RelPath {
				return a.filtered[i].RelPath < a.filtered[j].RelPath
			}
			return a.filtered[i].LineNumber < a.filtered[j].LineNumber
		})
	}
}

func (a *App) applyFilter(tag string) {
	a.filterTag = tag
	if tag == "" {
		a.filtered = append([]Annotation{}, a.all...)
	} else {
		a.filtered = nil
		for _, ann := range a.all {
			if ann.Tag == tag {
				a.filtered = append(a.filtered, ann)
			}
		}
	}
	a.sortFiltered()
	a.cursor = 0
	a.scroll = 0
}

func (a *App) cycleFilter() {
	if a.filterTag == "" {
		a.applyFilter(tags[0])
		return
	}
	for i, t := range tags {
		if t == a.filterTag {
			if i+1 < len(tags) {
				a.applyFilter(tags[i+1])
			} else {
				a.applyFilter("")
			}
			return
		}
	}
	a.applyFilter("")
}

func (a *App) handleCommand(cmd string) bool {
	cmd = strings.TrimSpace(strings.ToLower(cmd))
	switch cmd {
	case "/q", "/quit":
		return true
	case "/all":
		a.applyFilter("")
		a.message = "Showing all annotations"
	default:
		upper := strings.ToUpper(strings.TrimPrefix(cmd, "/"))
		found := false
		for _, t := range tags {
			if t == upper {
				a.applyFilter(t)
				a.message = fmt.Sprintf("Filtered: %s", t)
				found = true
				break
			}
		}
		if !found {
			a.message = fmt.Sprintf("Unknown command: %s", cmd)
		}
	}
	return false
}

func (a *App) openEditor() {
	if len(a.filtered) == 0 {
		return
	}
	ann := a.filtered[a.cursor]
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	var cmd *exec.Cmd
	if strings.Contains(editor, "vim") || strings.Contains(editor, "nvim") {
		cmd = exec.Command(editor, fmt.Sprintf("+%d", ann.LineNumber), ann.FilePath)
	} else if strings.Contains(editor, "nano") {
		cmd = exec.Command(editor, fmt.Sprintf("+%d", ann.LineNumber), ann.FilePath)
	} else if strings.Contains(editor, "code") {
		cmd = exec.Command(editor, "--goto", fmt.Sprintf("%s:%d", ann.FilePath, ann.LineNumber))
	} else {
		cmd = exec.Command(editor, ann.FilePath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func (a *App) render() {
	w, h := getTermSize()
	listHeight := h - 4 // header(2) + footer(2)
	var buf strings.Builder

	buf.WriteString(clearScr)

	// -- Header --
	title := " 📝 todo-tui"
	filterLabel := "ALL"
	if a.filterTag != "" {
		filterLabel = a.filterTag
	}
	sortLabel := "file"
	if a.sortByTag {
		sortLabel = "tag"
	}
	stats := fmt.Sprintf(" [%s|%s] %d annotations ", filterLabel, sortLabel, len(a.filtered))

	buf.WriteString(moveTo(1, 1))
	buf.WriteString(green + bold + title + reset)

	rootDisplay := "  " + a.root
	maxRootLen := w - len(title) - len(stats) - 2
	if maxRootLen > 0 && len(rootDisplay) > maxRootLen {
		rootDisplay = rootDisplay[:maxRootLen-1] + "…"
	}
	buf.WriteString(dim + rootDisplay + reset)

	if w > len(title)+len(stats) {
		buf.WriteString(moveTo(1, w-len(stats)))
		buf.WriteString(white + stats + reset)
	}

	// separator
	buf.WriteString(moveTo(2, 1))
	sep := strings.Repeat("─", w)
	buf.WriteString(dim + sep + reset)

	// -- List --
	if len(a.filtered) == 0 {
		buf.WriteString(moveTo(4, 3))
		buf.WriteString(dim + "No annotations found." + reset)
	} else {
		// adjust scroll
		if a.cursor < a.scroll {
			a.scroll = a.cursor
		}
		if a.cursor >= a.scroll+listHeight {
			a.scroll = a.cursor - listHeight + 1
		}

		for i := 0; i < listHeight; i++ {
			idx := a.scroll + i
			if idx >= len(a.filtered) {
				break
			}
			ann := a.filtered[idx]
			y := 3 + i
			isSelected := idx == a.cursor

			buf.WriteString(moveTo(y, 1))
			buf.WriteString(clearLine)

			tagStr := fmt.Sprintf(" %-5s", ann.Tag)
			locStr := fmt.Sprintf(" %s:%d", ann.RelPath, ann.LineNumber)
			descStr := ""
			if ann.Text != "" {
				descStr = "  " + ann.Text
			}

			// Truncate total line to terminal width
			fullLen := len(tagStr) + len(locStr) + len(descStr)
			if fullLen > w-1 {
				maxDesc := w - 1 - len(tagStr) - len(locStr) - 2
				if maxDesc > 3 {
					if len(descStr) > maxDesc {
						descStr = descStr[:maxDesc-1] + "…"
					}
				} else {
					descStr = ""
				}
			}

			if isSelected {
				buf.WriteString(reverse)
			}

			// tag with color
			color := tagColors[ann.Tag]
			if color == "" {
				color = white
			}
			if isSelected {
				buf.WriteString(reverse + bold + tagStr)
			} else {
				buf.WriteString(color + bold + tagStr + reset)
			}

			// location
			if isSelected {
				buf.WriteString(reverse + locStr)
			} else {
				buf.WriteString(locStr)
			}

			// description
			if isSelected {
				buf.WriteString(reverse + dim + descStr + reset)
			} else {
				buf.WriteString(dim + descStr + reset)
			}

			if isSelected {
				// Fill rest of line for full-row highlight
				filled := len(tagStr) + len(locStr) + len(descStr)
				if filled < w {
					buf.WriteString(reverse + strings.Repeat(" ", w-filled) + reset)
				} else {
					buf.WriteString(reset)
				}
			}
		}
	}

	// -- Footer --
	footerY := h - 1
	statusY := h

	buf.WriteString(moveTo(footerY, 1))
	buf.WriteString(dim + strings.Repeat("─", w) + reset)

	buf.WriteString(moveTo(statusY, 1))
	buf.WriteString(clearLine)
	if a.cmdMode {
		buf.WriteString(showCur)
		buf.WriteString(":" + a.cmdBuf)
	} else {
		buf.WriteString(hideCur)
		if a.message != "" {
			buf.WriteString(dim + " " + a.message + reset)
		} else {
			buf.WriteString(dim + " j/k:move  Enter:open  Tab:filter  s:sort  q:quit" + reset)
		}
	}

	fmt.Print(buf.String())
}

func (a *App) run() {
	state, err := enableRawMode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to enable raw mode: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		state.restore()
		fmt.Print(showCur + clearScr + moveTo(1, 1))
	}()

	fmt.Print(hideCur)

	for {
		a.render()
		key := readKey()

		if a.cmdMode {
			switch key {
			case "enter":
				quit := a.handleCommand(a.cmdBuf)
				a.cmdMode = false
				a.cmdBuf = ""
				if quit {
					return
				}
			case "esc":
				a.cmdMode = false
				a.cmdBuf = ""
			case "backspace":
				if len(a.cmdBuf) > 0 {
					a.cmdBuf = a.cmdBuf[:len(a.cmdBuf)-1]
				} else {
					a.cmdMode = false
				}
			default:
				if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
					a.cmdBuf += key
				}
			}
			continue
		}

		// Normal mode
		a.message = ""
		switch key {
		case "q":
			return
		case "/":
			a.cmdMode = true
			a.cmdBuf = "/"
		case "up", "k":
			if a.cursor > 0 {
				a.cursor--
			}
		case "down", "j":
			if a.cursor < len(a.filtered)-1 {
				a.cursor++
			}
		case "s":
			a.sortByTag = !a.sortByTag
			a.sortFiltered()
			a.cursor = 0
			a.scroll = 0
			if a.sortByTag {
				a.message = "Sort: by tag type"
			} else {
				a.message = "Sort: by file"
			}
		case "tab":
			a.cycleFilter()
		case "enter":
			// Restore terminal, open editor, then re-enable raw mode
			state.restore()
			fmt.Print(showCur + clearScr + moveTo(1, 1))
			a.openEditor()
			state2, err := enableRawMode()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to re-enable raw mode: %v\n", err)
				return
			}
			state = state2
			fmt.Print(hideCur)
		}
	}
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: '%s' is not a directory\n", root)
		os.Exit(1)
	}

	fmt.Printf("Scanning %s …\n", absRoot)
	annotations := scanDirectory(absRoot)
	fmt.Printf("Found %d annotations.\n", len(annotations))

	if len(annotations) == 0 {
		fmt.Println("No TODO/FIXME/HACK/NOTE annotations found.")
		os.Exit(0)
	}

	app := newApp(annotations, absRoot)
	app.run()
}
