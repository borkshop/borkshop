package main

import (
	"borkshop/bottle"
	"borkshop/bottlemudslide"
	"borkshop/bottlepid"
	"borkshop/bottlesimstats"
	"borkshop/bottletectonic"
	"borkshop/bottleview"
	"borkshop/bottleviewearth"
	"borkshop/bottleviewplate"
	"borkshop/bottleviewtopo"
	"borkshop/bottleviewwater"
	"borkshop/bottlewatercoverage"
	"borkshop/bottlewatershed"
	"borkshop/hilbert"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/jcorbin/anansi/ansi"
	"github.com/jcorbin/anansi/x/platform"
)

var errInt = errors.New("interrupt")

func main() {
	rand.Seed(time.Now().UnixNano()) // TODO find the right place to seed
	// TODO load config from file
	flag.Parse()
	platform.MustRun(os.Stdout, func(p *platform.Platform) error {
		for {
			if err := p.Run(newView()); platform.IsReplayDone(err) {
				continue // loop replay
			} else if err == io.EOF || err == errInt {
				return nil
			} else if err != nil {
				log.Printf("exiting due to %v", err)
				return err
			}
		}
	}, platform.FrameRate(60), platform.Config{
		LogFileName: "ansimata.log",
	})
}

func newView() *view {
	const scale = 256
	rect := image.Rect(0, 0, scale, scale)
	plates := &bottletectonic.Plates{
		Scale: hilbert.Scale(scale),
	}
	quakes := &bottletectonic.Quakes{
		Scale:     hilbert.Scale(scale),
		Magnitude: 1,
		Controller: bottlepid.Controller{
			Proportional: bottlepid.G(0xfff, 1),
			Integral:     bottlepid.G(0xf, 1),
			Differential: bottlepid.G(0xf, 1),
			Value:        50,
			Min:          0,
			Max:          0xffff,
		},
		// Disabled:  true,
	}
	mudSlide := &bottlemudslide.Simulation{
		Scale:  hilbert.Scale(scale),
		Repose: 2,
	}
	watershed := &bottlewatershed.Simulation{
		Scale: hilbert.Scale(scale),
	}
	waterCoverage := &bottlewatercoverage.Simulation{
		Controller: bottlepid.Controller{
			Proportional: bottlepid.G(0xff, 1),
			Integral:     bottlepid.G(1, 1),
			Differential: bottlepid.G(1, 1),
			Value:        scale * scale / 2,
			Min:          -0xffffffff,
			Max:          0xffffffff,
		},
	}
	res := bottle.Resetters{
		// bottletower.Resetter{Scale: scale},
		// bottletoposimplex.New(scale),
		bottletectonic.Resetter{},
		// bottleflood.New(scale, 0),
	}
	next := bottle.NewGeneration(scale)
	prev := bottle.NewGeneration(scale)
	res.Reset(prev)
	ticker := bottle.Tickers{
		bottlesimstats.Pre{},
		mudSlide,
		plates,
		quakes,
		watershed,
		waterCoverage,
		bottlesimstats.Post{},
	}

	_ = plates
	_ = quakes
	_ = watershed

	// Views
	topoView := bottleviewtopo.New(scale)
	plateView := bottleviewplate.New(scale)
	earthView := bottleviewearth.New(scale)
	waterView := bottleviewwater.New(scale)

	return &view{
		rect:          rect,
		ticker:        ticker,
		resetter:      res,
		waterCoverage: waterCoverage,
		quakes:        quakes,
		prev:          prev,
		next:          next,

		earthView: earthView,
		waterView: waterView,
		plateView: plateView,
		topoView:  topoView,
		view:      topoView,
	}
}

type view struct {
	rect       image.Rectangle
	ticker     bottle.Ticker
	resetter   bottle.Resetter
	next, prev *bottle.Generation

	ticking int

	waterCoverage *bottlewatercoverage.Simulation
	quakes        *bottletectonic.Quakes

	view      bottleview.View
	earthView bottleview.View
	waterView bottleview.View
	plateView bottleview.View
	topoView  bottleview.View
}

func (v *view) Update(ctx *platform.Context) (err error) {
	// Ctrl-C interrupts
	if ctx.Input.HasTerminal('\x03') {
		err = errInt
	}

	// Ctrl-Z suspends
	if ctx.Input.CountRune('\x1a') > 0 {
		defer func() {
			if err == nil {
				err = ctx.Suspend()
			} // else NOTE don't bother suspending, e.g. if Ctrl-C was also present
		}()
	}

	switch {
	case ctx.Input.CountRune('E') > 0:
		v.view = v.earthView
	case ctx.Input.CountRune('W') > 0:
		v.view = v.waterView
	case ctx.Input.CountRune('P') > 0:
		v.view = v.plateView
	case ctx.Input.CountRune('T') > 0:
		v.view = v.topoView
	}

	v.ticking += ctx.Input.CountRune('p')
	var ticks int
	if v.ticking%2 == 0 {
		ticks = 1
	}
	ticks += ctx.Input.CountRune('n')

	if ctx.Input.CountRune('r') > 0 {
		v.resetter.Reset(v.prev)
		v.resetter.Reset(v.next)
	}

	for i := 0; i < ticks; i++ {
		v.ticker.Tick(v.next, v.prev)
		v.next, v.prev = v.prev, v.next
	}

	v.view.Draw(ctx.Output, ctx.Output.Grid.Rect, v.next, image.ZP)

	gen := v.next
	screen := ctx.Output
	screen.To(ansi.Pt(1, 1))
	screen.WriteString(fmt.Sprintf("EarthElevation %d...%f...%d\r\n", gen.EarthElevationStats.Min, gen.EarthElevationStats.Mean(), gen.EarthElevationStats.Max))
	screen.WriteString(fmt.Sprintf("WaterElevation %d...%f...%d\r\n", gen.WaterElevationStats.Min, gen.WaterElevationStats.Mean(), gen.WaterElevationStats.Max))
	screen.WriteString(fmt.Sprintf("Water %d...%f...%d\r\n", gen.WaterStats.Min, gen.WaterStats.Mean(), gen.WaterStats.Max))
	screen.WriteString(fmt.Sprintf("WaterCoverage %d\r\n", gen.WaterCoverage))
	screen.WriteString(fmt.Sprintf("     Converge %d\r\n", v.waterCoverage.Controller.Value))
	screen.WriteString(fmt.Sprintf(" C %d\r\n", gen.WaterCoverageController.Proportional))
	screen.WriteString(fmt.Sprintf(" P %d\r\n", gen.WaterCoverageController.Integral))
	screen.WriteString(fmt.Sprintf(" I %d\r\n", gen.WaterCoverageController.Differential))
	screen.WriteString(fmt.Sprintf(" D %d\r\n", gen.WaterCoverageController.Control))
	screen.WriteString(fmt.Sprintf("ElevationSpread %d\r\n", gen.EarthElevationStats.Spread()))
	screen.WriteString(fmt.Sprintf("       Converge %d\r\n", v.quakes.Controller.Value))
	screen.WriteString(fmt.Sprintf(" C %d\r\n", gen.ElevationSpreadController.Proportional))
	screen.WriteString(fmt.Sprintf(" P %d\r\n", gen.ElevationSpreadController.Integral))
	screen.WriteString(fmt.Sprintf(" I %d\r\n", gen.ElevationSpreadController.Differential))
	screen.WriteString(fmt.Sprintf(" D %d\r\n", gen.ElevationSpreadController.Control))
	screen.WriteString(fmt.Sprintf("WaterFlow %d\r\n", gen.WaterFlow))
	screen.WriteString(fmt.Sprintf("EarthFlow %d\r\n", gen.EarthFlow))
	screen.WriteString(fmt.Sprintf("QuakeFlow %d\r\n", gen.QuakeFlow))
	for i := 0; i < bottle.NumPlates; i++ {
		screen.WriteString(fmt.Sprintf("Plate[%d] %d\r\n", i, gen.PlateSizes[i]))
	}

	return
}
