package cache

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
)

const progressMaxWidth = 80

var (
	quiet bool
	pink  = lipgloss.Color("#FF7CCB")
	gold  = lipgloss.Color("#FDFF8C")
)

func SetQuiet(q bool) { quiet = q }

type Progress struct {
	bar     progress.Model
	label   string
	total   int64
	current int64
	mu      sync.Mutex
}

func NewProgress(label string, total int) *Progress {
	if total < 1 {
		total = 1
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
	}
}

func (p *Progress) Start() {
	if quiet {
		return
	}
	atomic.StoreInt64(&p.current, 0)
	p.mu.Lock()
	fmt.Printf("%s: %s", p.label, p.bar.ViewAs(0))
	_ = os.Stdout.Sync()
	p.mu.Unlock()
}

func (p *Progress) Add() {
	cur := atomic.AddInt64(&p.current, 1)
	if quiet {
		return
	}
	percent := float64(cur) / float64(p.total)
	if percent > 1.0 {
		percent = 1.0
	}
	view := p.bar.ViewAs(percent)
	p.mu.Lock()
	fmt.Printf("\r%s: %s", p.label, view)
	_ = os.Stdout.Sync()
	p.mu.Unlock()
}

func (p *Progress) Finish() {
	if quiet {
		return
	}
	fmt.Println()
}
