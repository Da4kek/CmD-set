package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Retro cyberpunk amber palette
const (
	Amber    = lipgloss.Color("#FF9500")
	AmberDim = lipgloss.Color("#7A4700")
	AmberBg  = lipgloss.Color("#1A0E00") // very dark amber tint for selected bg
	Neon     = lipgloss.Color("#00FF9F")
	Pink     = lipgloss.Color("#FF2D78")
	Cyan     = lipgloss.Color("#00D4FF")
	Red      = lipgloss.Color("#FF453A")
	Yellow   = lipgloss.Color("#FFE600")
	Muted    = lipgloss.Color("#484848")
	Muted2   = lipgloss.Color("#2E2E2E")
	TextMain = lipgloss.Color("#E0D5C0")
	Black    = lipgloss.Color("#000000")
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Amber)

	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Black).
			Background(Amber).
			Padding(0, 1)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(Muted).
				Padding(0, 1)

	StatusBarStyle = lipgloss.NewStyle().
			Background(Muted2).
			Foreground(Muted).
			Padding(0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Amber)

	NormalItemStyle = lipgloss.NewStyle().
			Foreground(TextMain)

	TagStyle = lipgloss.NewStyle().
			Foreground(Neon)

	DimStyle = lipgloss.NewStyle().
			Foreground(Muted)

	AccentStyle = lipgloss.NewStyle().
			Foreground(Cyan)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Neon)

	WarnStyle = lipgloss.NewStyle().
			Foreground(Yellow)

	PinkStyle = lipgloss.NewStyle().
			Foreground(Pink)

	ActiveFieldStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Black).
				Background(Amber).
				Padding(0, 1)

	SepStyle = lipgloss.NewStyle().
			Foreground(Muted2)
)

func Sep(width int) string {
	if width <= 0 {
		return ""
	}
	return SepStyle.Render(strings.Repeat("─", width))
}

func FieldLabel(name string, active bool) string {
	padded := "  " + name + " "
	if active {
		return ActiveFieldStyle.Render(padded)
	}
	return DimStyle.Render(padded)
}
