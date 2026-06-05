package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// Format represents output format
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Printer handles structured output
type Printer struct {
	format Format
	w      io.Writer
}

// NewPrinter creates a new output printer
func NewPrinter(format string, w io.Writer) *Printer {
	f := FormatText
	if format == "json" {
		f = FormatJSON
	}
	return &Printer{format: f, w: w}
}

// Print outputs data in the configured format
func (p *Printer) Print(v any) error {
	if p.format == FormatJSON {
		enc := json.NewEncoder(p.w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	return p.printText(v)
}

// PrintString prints a plain string (text mode only)
func (p *Printer) PrintString(s string) {
	if p.format == FormatText {
		fmt.Fprint(p.w, s)
	}
}

// Printf prints formatted text (text mode only)
func (p *Printer) Printf(format string, args ...any) {
	if p.format == FormatText {
		fmt.Fprintf(p.w, format, args...)
	}
}

func (p *Printer) printText(v any) error {
	switch data := v.(type) {
	case string:
		fmt.Fprintln(p.w, data)
	case []map[string]any:
		for _, item := range data {
			for k, v := range item {
				fmt.Fprintf(p.w, "%s: %v\n", k, v)
			}
			fmt.Fprintln(p.w)
		}
	default:
		fmt.Fprintf(p.w, "%+v\n", v)
	}
	return nil
}
