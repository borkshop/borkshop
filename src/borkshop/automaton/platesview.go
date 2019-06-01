package main

import (
	"image"
	"image/color"

	"github.com/hsluv/hsluv-go"
	"github.com/jcorbin/anansi"
	"github.com/jcorbin/anansi/ansi"
)

type AnansiPlatesView struct {
	automaton *Automaton
	image     *image.RGBA
	colors    []color.RGBA
}

func NewAnansiPlatesView(automaton *Automaton) *AnansiPlatesView {
	image := image.NewRGBA(automaton.rect)
	colors := make([]color.RGBA, automaton.numPlates)
	writePlateColors(colors)

	return &AnansiPlatesView{
		automaton: automaton,
		image:     image,
		colors:    colors,
	}
}

func (v *AnansiPlatesView) Draw(screen *anansi.Screen, rect ansi.Rectangle) {
	// TODO offset point
	drawPlates(v.image, v.automaton.plates, v.automaton.points, v.colors)
	drawAnansi(screen, rect, v.image)
}

func writePlateColors(dst []color.RGBA) {
	count := len(dst)
	for i := 0; i < count; i++ {
		dst[i] = newHueFractionColor(i, count)
	}
}

func newHueFractionColor(over, under int) color.RGBA {
	r, g, b := hsluv.HsluvToRGB(
		360*float64(over)/float64(under),
		100,
		30,
	)
	return color.RGBA{
		uint8(r * 0xff),
		uint8(g * 0xff),
		uint8(b * 0xff),
		0xff,
	}
}