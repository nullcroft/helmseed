package cache

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
)

const progressMaxWidth = 80

var (
	pink = lipgloss.Color("#FF7CCB")
	gold = lipgloss.Color("#FDFF8C")
)

type Progress struct {
	bar     progress.Model
	label   string
	total   int64
	current int64
	out     io.Writer
	quiet   bool
	mu      sync.Mutex
}

func NewProgress(label string, total int, out io.Writer, quiet bool) *Progress {
	if total < 1 {
		total = 1
	}
	if out == nil {
		out = os.Stdout
	}
	bar := progress.New(
		progress.WithScaled(true),
		progress.WithColors(pink, gold),
		progress.WithoutPercentage(),
		progress.WithWidth(progressMaxWidth),
	)
	return &Progress{
		bar:   bar,
		label: label,
		total: int64(total),
		out:   out,
		quiet: quiet,
	}
}

func (p *Progress) Start() {
	if p.quiet {
		return
	}
	atomic.StoreInt64(&p.current, 0)
	p.mu.Lock()
	_, _ = fmt.Fprintf(p.out, "%s: %s", p.label, p.bar.ViewAs(0))
	p.mu.Unlock()
}

func (p *Progress) Add() {
	cur := atomic.AddInt64(&p.current, 1)
	if p.quiet {
		return
	}
	percent := float64(cur) / float64(p.total)
	if percent > 1.0 {
		percent = 1.0
	}
	p.mu.Lock()
	_, _ = fmt.Fprintf(p.out, "\r%s: %s", p.label, p.bar.ViewAs(percent))
	p.mu.Unlock()
}

func (p *Progress) Finish() {
	if p.quiet {
		return
	}
	_, _ = fmt.Fprintln(p.out)
}
