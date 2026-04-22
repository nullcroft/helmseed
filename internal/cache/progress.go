package cache

import (
	"fmt"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/progress"
)

var quiet bool

func SetQuiet(q bool) { quiet = q }

type Progress struct {
	model   progress.Model
	label   string
	total   int64
	current int64
}

func NewProgress(label string, total int) *Progress {
	if total < 1 {
		total = 1
	}
	m := progress.New(
		progress.WithSolidFill("█"),
		progress.WithoutPercentage(),
		progress.WithWidth(40),
	)
	return &Progress{
		model: m,
		label:  label,
		total:  int64(total),
	}
}

func (p *Progress) Start() {
	if quiet {
		return
	}
	atomic.StoreInt64(&p.current, 0)
	fmt.Print(p.label + ": ")
	fmt.Print(p.model.ViewAs(0))
}

func (p *Progress) Add() {
	cur := atomic.AddInt64(&p.current, 1)
	if quiet {
		return
	}
	percent := float64(cur) / float64(p.total)
	view := p.model.ViewAs(percent)
	fmt.Print("\r" + p.label + ": " + view)
}

func (p *Progress) Finish() {
	if quiet {
		return
	}
	fmt.Println()
}