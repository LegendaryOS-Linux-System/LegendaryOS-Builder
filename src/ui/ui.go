package ui

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

const (
	reset     = "\033[0m"
	bold      = "\033[1m"
	fgRed     = "\033[91m"
	fgGreen   = "\033[92m"
	fgYellow  = "\033[93m"
	fgBlue    = "\033[94m"
	fgMagenta = "\033[95m"
	fgCyan    = "\033[96m"
	fgWhite   = "\033[97m"
	fgGray    = "\033[90m"
)

var Out io.Writer = os.Stderr

func c(codes ...string) string { return strings.Join(codes, "") }

func Banner() {
	fmt.Fprint(Out, "\n")
	lines := []string{
		"  ██╗     ███████╗ ██████╗ ███████╗███╗   ██╗██████╗  █████╗ ██████╗ ██╗   ██╗ ██████╗ ███████╗",
		"  ██║     ██╔════╝██╔════╝ ██╔════╝████╗  ██║██╔══██╗██╔══██╗██╔══██╗╚██╗ ██╔╝██╔═══██╗██╔════╝",
		"  ██║     █████╗  ██║  ███╗█████╗  ██╔██╗ ██║██║  ██║███████║██████╔╝ ╚████╔╝ ██║   ██║███████╗",
		"  ██║     ██╔══╝  ██║   ██║██╔══╝  ██║╚██╗██║██║  ██║██╔══██║██╔══██╗  ╚██╔╝  ██║   ██║╚════██║",
		"  ███████╗███████╗╚██████╔╝███████╗██║ ╚████║██████╔╝██║  ██║██║  ██║   ██║   ╚██████╔╝███████║",
		"  ╚══════╝╚══════╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚══════╝",
	}
	for _, l := range lines {
		fmt.Fprintln(Out, c(fgCyan, bold)+l+reset)
	}
	fmt.Fprintln(Out)
	fmt.Fprintln(Out, c(fgGray)+"                    OS Builder  ·  Fedora Edition  ·  x86_64  ·  bootc"+reset)
	fmt.Fprintln(Out)
	fmt.Fprintln(Out, c(fgGray)+"  "+strings.Repeat("─", 80)+reset)
	fmt.Fprintln(Out)
}

func SmallBanner() {
	fmt.Fprint(Out, "\n")
	fmt.Fprintf(Out, "  %s⬡ LegendaryOS Builder%s\n", c(fgCyan, bold), reset)
	fmt.Fprintf(Out, "  %s\n\n", c(fgGray)+strings.Repeat("─", 48)+reset)
}

func Section(title string) {
	fmt.Fprintln(Out)
	fmt.Fprintf(Out, "  %s› %s%s\n", c(fgBlue, bold), strings.ToUpper(title), reset)
	fmt.Fprintf(Out, "  %s\n", c(fgGray)+strings.Repeat("─", 48)+reset)
}

func Step(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(Out, "  %s⬡%s %s%s%s\n", c(fgBlue, bold), reset, c(fgWhite, bold), msg, reset)
}

func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(Out, "    %s·%s %s%s%s\n", c(fgGray), reset, c(fgWhite), msg, reset)
}

func OK(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(Out, "  %s✓%s %s%s%s\n", c(fgGreen, bold), reset, c(fgWhite), msg, reset)
}

func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(Out, "  %s⚠%s %s%s%s\n", c(fgYellow, bold), reset, c(fgYellow), msg, reset)
}

func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(Out, "  %s✗%s %s%s%s\n", c(fgRed, bold), reset, c(fgWhite, bold), msg, reset)
}

func Fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(Out)
	fmt.Fprintf(Out, "  %s%s%s\n", c(fgRed, bold), strings.Repeat("━", 54), reset)
	fmt.Fprintf(Out, "  %s✗  BUILD FAILED%s\n", c(fgRed, bold), reset)
	fmt.Fprintf(Out, "  %s%s%s\n\n", c(fgRed, bold), strings.Repeat("━", 54), reset)
	fmt.Fprintf(Out, "  %sError:%s %s%s%s\n\n", c(fgRed), reset, c(fgWhite, bold), msg, reset)
	fmt.Fprintf(Out, "  %sTip: run with --verbose / -v for full output%s\n\n", c(fgGray), reset)
	os.Exit(1)
}

func Newline() { fmt.Fprintln(Out) }
func Divider() {
	fmt.Fprintf(Out, "  %s%s%s\n", c(fgGray), strings.Repeat("─", 54), reset)
}

// ── Progress bar ─────────────────────────────────────────────────────────────

type ProgressBar struct {
	Total   int
	Current int
	Label   string
	Width   int
	start   time.Time
}

func NewProgressBar(total int, label string) *ProgressBar {
	return &ProgressBar{Total: total, Label: label, Width: 34, start: time.Now()}
}

func (p *ProgressBar) Set(n int) { p.Current = n; p.render() }
func (p *ProgressBar) Inc()      { p.Current++; p.render() }
func (p *ProgressBar) Done() {
	p.Current = p.Total
	p.render()
	fmt.Fprintln(Out)
}

func (p *ProgressBar) render() {
	pct := 0.0
	if p.Total > 0 {
		pct = float64(p.Current) / float64(p.Total)
	}
	filled := int(math.Round(float64(p.Width) * pct))
	if filled > p.Width {
		filled = p.Width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.Width-filled)
	elapsed := time.Since(p.start).Round(time.Second)
	fmt.Fprintf(Out, "\r  %s⚡%s %-22s %s[%s]%s %s%3.0f%%%s  %s%s%s   ",
		c(fgBlue, bold), reset, p.Label,
		c(fgCyan), bar, reset,
		c(fgWhite, bold), pct*100, reset,
		c(fgGray), elapsed, reset,
	)
}

// ── Build summary ─────────────────────────────────────────────────────────────

type SummaryStep struct {
	Name   string
	Status string // ok | warn | skip | fail
	Detail string
}

type BuildSummary struct {
	ProjectName string
	Version     string
	Distro      string
	Steps       []SummaryStep
	ImageTag    string
	ISOPath     string
	Duration    time.Duration
}

func PrintBuildSummary(s *BuildSummary) {
	fmt.Fprintln(Out)
	fmt.Fprintf(Out, "  %s%s%s\n", c(fgCyan, bold), strings.Repeat("═", 54), reset)
	fmt.Fprintf(Out, "  %s★  BUILD COMPLETE%s\n", c(fgCyan, bold), reset)
	fmt.Fprintf(Out, "  %s%s%s\n\n", c(fgCyan, bold), strings.Repeat("═", 54), reset)
	kv := func(k, v string) {
		fmt.Fprintf(Out, "  %s%-14s%s %s%s%s\n", c(fgGray), k+":", reset, c(fgWhite, bold), v, reset)
	}
	kv("Project", s.ProjectName+" "+s.Version)
	kv("Base", s.Distro)
	kv("Duration", s.Duration.Round(time.Second).String())
	fmt.Fprintln(Out)
	fmt.Fprintf(Out, "  %sSteps:%s\n", c(fgGray), reset)
	for _, st := range s.Steps {
		switch st.Status {
		case "ok":
			fmt.Fprintf(Out, "    %s✓%s %-32s %s%s%s\n", c(fgGreen, bold), reset, st.Name, c(fgGray), st.Detail, reset)
		case "warn":
			fmt.Fprintf(Out, "    %s⚠%s %-32s %s%s%s\n", c(fgYellow, bold), reset, st.Name, c(fgGray), st.Detail, reset)
		case "skip":
			fmt.Fprintf(Out, "    %s·%s %-32s %s(skipped)%s\n", c(fgGray), reset, st.Name, c(fgGray), reset)
		case "fail":
			fmt.Fprintf(Out, "    %s✗%s %-32s %s%s%s\n", c(fgRed, bold), reset, st.Name, c(fgRed), st.Detail, reset)
		}
	}
	if s.ImageTag != "" {
		fmt.Fprintln(Out)
		fmt.Fprintf(Out, "  %s%s%s\n", c(fgGray), strings.Repeat("─", 54), reset)
		fmt.Fprintf(Out, "  %s◈ Image:%s %s%s%s\n", c(fgCyan, bold), reset, c(fgWhite, bold), s.ImageTag, reset)
	}
	if s.ISOPath != "" {
		fmt.Fprintln(Out)
		fmt.Fprintf(Out, "  %s%s%s\n", c(fgGray), strings.Repeat("─", 54), reset)
		fmt.Fprintf(Out, "  %s◈ ISO:%s %s%s%s\n", c(fgCyan, bold), reset, c(fgWhite, bold), s.ISOPath, reset)
	}
	fmt.Fprintln(Out)
}

// ── Input helpers ─────────────────────────────────────────────────────────────

func AskDefault(question, def string) string {
	fmt.Fprintf(Out, "  %s›%s %s%s%s %s[%s]%s: ",
		c(fgMagenta, bold), reset, c(fgWhite, bold), question, reset, c(fgGray), def, reset)
	var ans string
	fmt.Scanln(&ans)
	if strings.TrimSpace(ans) == "" {
		return def
	}
	return strings.TrimSpace(ans)
}

func AskYN(question string, def bool) bool {
	opts := "[Y/n]"
	if !def {
		opts = "[y/N]"
	}
	fmt.Fprintf(Out, "  %s›%s %s%s%s %s%s%s: ",
		c(fgMagenta, bold), reset, c(fgWhite, bold), question, reset, c(fgGray), opts, reset)
	var ans string
	fmt.Scanln(&ans)
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans == "" {
		return def
	}
	return ans == "y" || ans == "yes"
}

func AskChoice(question string, choices []string, def int) int {
	fmt.Fprintf(Out, "  %s›%s %s%s%s\n", c(fgMagenta, bold), reset, c(fgWhite, bold), question, reset)
	for i, ch := range choices {
		if i == def {
			fmt.Fprintf(Out, "    %s[%d] %s%s\n", c(fgGreen, bold), i+1, ch, reset)
		} else {
			fmt.Fprintf(Out, "    %s[%d]%s %s\n", c(fgGray), i+1, reset, ch)
		}
	}
	fmt.Fprintf(Out, "  %s›%s Choice %s[%d]%s: ", c(fgMagenta, bold), reset, c(fgGray), def+1, reset)
	var n int
	fmt.Scan(&n)
	if n < 1 || n > len(choices) {
		return def
	}
	return n - 1
}

func PackageListDisplay(title string, pkgs []string) {
	fmt.Fprintf(Out, "  %s⊕%s %s%s%s %s(%d)%s\n",
		c(fgBlue, bold), reset, c(fgWhite, bold), title, reset, c(fgGray), len(pkgs), reset)
	cols := 3
	for i, p := range pkgs {
		if i%cols == 0 {
			fmt.Fprint(Out, "    ")
		}
		fmt.Fprintf(Out, "%s%-28s%s", c(fgWhite), p, reset)
		if i%cols == cols-1 || i == len(pkgs)-1 {
			fmt.Fprintln(Out)
		}
	}
}
