// +build js

package main

import (
	"borkshop/stats"
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"syscall/js"
	"time"
)

var (
	document          = js.Global().Get("document")
	window            = js.Global().Get("window")
	ImageData         = js.Global().Get("ImageData")
	Uint8ClampedArray = js.Global().Get("Uint8ClampedArray")
)

const timingWindow = 4 * 60

type Updater interface {
	Update(*imContext) error
}

type Opener interface {
	Open() (io.Closer, error)
}

type imContext struct {
	client Updater

	// timing
	now          time.Time
	elapsed      time.Duration
	frameTimes   stats.Times
	updateTimes  stats.Durations
	clientTimes  stats.Durations
	renderTimes  stats.Durations
	elapsedTimes stats.Durations

	// TODO animation/simulation time
	imInput
	imOutput

	// animation
	animating bool
	lastFrame time.Time
	rafBase   time.Time
	rafHandle js.Value
	rafFn     js.Func

	// dom bindings
	canvas    js.Value
	renderCtx js.Value

	infoDetails js.Value
	infoBody    js.Value

	profTiming  bool
	profDetails js.Value
	profTitle   js.Value
	profBody    js.Value

	// run done
	done chan error
}

type keyMod uint8

const (
	altKey keyMod = 1 << iota
	ctrlKey
	metaKey
	shiftKey
)

func readKeyMod(event js.Value) keyMod {
	var mod keyMod
	if event.Get("altKey").Bool() {
		mod |= altKey
	}
	if event.Get("ctrlKey").Bool() {
		mod |= ctrlKey
	}
	if event.Get("shiftKey").Bool() {
		mod |= metaKey
	}
	if event.Get("metaKey").Bool() {
		mod |= shiftKey
	}
	return mod
}

type imInput struct {
	key struct {
		press rune
		mod   keyMod
		// TODO down buttons
	}
	// TODO mouse struct {}
}

type imOutput struct {
	screen *image.RGBA // TODO clarify screen-space vs cell-space
	prof   bytes.Buffer
	info   bytes.Buffer
}

func (ctx *imContext) Run(client Updater) (err error) {
	ctx.client = client
	if op, ok := client.(Opener); ok {
		var cl io.Closer
		cl, err = op.Open()
		defer func() {
			if cerr := cl.Close(); err == nil {
				err = cerr
			}
		}()
	}

	err = ctx.init()
	defer ctx.release()

	if err == nil {
		err = <-ctx.done
	}
	return err
}

func (ctx *imContext) init() (err error) {
	ctx.canvas, err = getEnvSelector("canvas")
	if err != nil {
		return err
	}

	ctx.infoDetails, err = getEnvSelector("info-details")
	if err != nil {
		return err
	}

	ctx.profDetails, err = getEnvSelector("prof-details")
	if err != nil {
		return err
	}

	ctx.frameTimes = stats.MakeTimes(timingWindow)
	ctx.updateTimes = stats.MakeDurations(timingWindow)
	ctx.renderTimes = stats.MakeDurations(timingWindow)
	ctx.clientTimes = stats.MakeDurations(timingWindow)
	ctx.elapsedTimes = stats.MakeDurations(timingWindow)

	ctx.rafFn = js.FuncOf(ctx.onFrame)

	ctx.infoBody = ctx.infoDetails.Call("appendChild", document.Call("createElement", "pre"))
	ctx.profTitle = ctx.profDetails.Call("querySelector", "summary")
	ctx.profBody = ctx.profDetails.Call("appendChild", document.Call("createElement", "pre"))

	// TODO webgl instead
	// TODO initialize cell rendering gl program
	ctx.renderCtx = ctx.canvas.Call("getContext", "2d")

	parent := ctx.canvas.Get("parentNode")
	parent.Call("addEventListener", "keypress", js.FuncOf(ctx.onKeyPress))
	window.Call("addEventListener", "resize", js.FuncOf(ctx.onResize))

	ctx.done = make(chan error)

	if ctx.animating {
		ctx.requestFrame()
	}
	ctx.updateSize()

	return nil
}

func (ctx *imContext) requestFrame() {
	ctx.rafHandle = js.Global().Call("requestAnimationFrame", ctx.rafFn)
}

func (ctx *imContext) cancelFrame() {
	if ctx.rafHandle != js.Undefined() {
		js.Global().Call("cancelAnimationFrame", ctx.rafHandle)
		ctx.rafHandle = js.Undefined()
		ctx.lastFrame = time.Time{}
	}
}

func (ctx *imContext) onFrame(this js.Value, args []js.Value) interface{} {
	ctx.rafHandle = js.Undefined()
	if !ctx.animating {
		return nil
	}

	millisec := args[0].Float()
	rafRel := time.Duration(millisec * 1e6)
	if ctx.rafBase.IsZero() {
		ctx.rafBase = time.Now().Add(-rafRel)
	}
	now := ctx.rafBase.Add(rafRel)

	ctx.now = now
	if !ctx.lastFrame.IsZero() {
		ctx.elapsed = now.Sub(ctx.lastFrame)
		ctx.elapsedTimes.Collect(ctx.elapsed)
	}
	ctx.Update()
	ctx.requestFrame()
	ctx.lastFrame = now
	return nil
}

func (ctx *imContext) onResize(this js.Value, args []js.Value) interface{} {
	ctx.updateSize()
	ctx.Update()
	return nil
}

func (ctx *imContext) updateSize() {
	parent := ctx.canvas.Get("parentNode")
	size := image.Pt(
		parent.Get("clientWidth").Int(),
		parent.Get("clientHeight").Int(),
	)

	// TODO decouple grid size from screen size

	// TODO reuse prior capacity when possible
	ctx.screen = image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
}

func (ctx *imContext) onKeyPress(this js.Value, args []js.Value) interface{} {
	ctx.imInput.onKeyPress(this, args)
	ctx.Update()
	return nil
}

func (ctx *imContext) release() {
	ctx.cancelFrame()
	ctx.rafFn.Release()
}

func (ctx *imContext) Update() {
	defer ctx.updateTimes.Measure()()

	if ctx.now.IsZero() {
		ctx.now = time.Now()
	}

	// render when not animating or animation has advanced
	if !ctx.animating || ctx.elapsed > 0 {
		ctx.frameTimes.Collect(ctx.now)
		defer ctx.Render()
	}

	defer func(wereAnimating bool) {
		// request or cancel next frame as needed
		if wereAnimating && !ctx.animating {
			ctx.cancelFrame()
		} else if !wereAnimating && ctx.animating {
			ctx.requestFrame()
		}

		// clear one-shot state
		ctx.now = time.Time{}
		ctx.elapsed = 0
		ctx.clearInput()
	}(ctx.animating)

	if ctx.key.press == 'q' && ctx.key.mod == ctrlKey {
		ctx.clearInput()
		ctx.done <- nil
		return
	}

	if ctx.key.press == 'p' && ctx.key.mod == ctrlKey {
		ctx.clearInput()
		ctx.profTiming = !ctx.profTiming
		if !ctx.profTiming {
			ctx.clearProf()
		}
	}

	if ctx.profTiming {
		ctx.clearProf()
		ctx.proff("%v FPS\n", ctx.frameTimes.CountRecent(ctx.now, time.Second))
	}

	ctx.updateClient()

	if ctx.profTiming {
		ctx.proff("µ update: %v\n", ctx.updateTimes.Average())
		ctx.proff("µ client: %v\n", ctx.clientTimes.Average())
		ctx.proff("µ render: %v\n", ctx.renderTimes.Average())
		ctx.proff("µ 𝝙frame: %v\n", ctx.elapsedTimes.Average())
	}
}

func (ctx *imContext) updateClient() {
	defer ctx.clientTimes.Measure()()
	if err := ctx.client.Update(ctx); err != nil {
		ctx.done <- err
	}
}

func (ctx *imContext) Render() {
	defer ctx.renderTimes.Measure()()

	// update profiling details
	if ctx.prof.Len() == 0 {
		ctx.profDetails.Get("style").Set("display", "none")
		ctx.profTitle.Set("innerText", "")
		ctx.profBody.Set("innerText", "")
	} else {
		ctx.profDetails.Get("style").Set("display", "")
		if ctx.profDetails.Get("open").Bool() {
			ctx.profTitle.Set("innerText", "")
			ctx.profBody.Set("innerText", ctx.prof.String())
		} else {
			b := ctx.prof.Bytes()
			if i := bytes.IndexByte(b, '\n'); i > 0 {
				b = b[:i]
			}
			ctx.profTitle.Set("innerText", string(b))
			ctx.profBody.Set("innerText", "")
		}
	}

	// update simulation info details
	ctx.infoBody.Set("innerText", ctx.info.String())

	// render the world grid
	size := ctx.screen.Rect.Size()
	ar := js.TypedArrayOf(ctx.screen.Pix)
	defer ar.Release()

	// TODO can we just retain this image object between renders?
	img := ImageData.New(Uint8ClampedArray.New(ar), size.X, size.Y)

	ctx.renderCtx.Call("putImageData", img, 0, 0)
}

func (in *imInput) clearInput() {
	in.key.press = 0
}

func (in *imInput) onKeyPress(this js.Value, args []js.Value) interface{} {
	event := args[0]
	in.key.mod = readKeyMod(event)
	for _, r := range event.Get("key").String() {
		in.key.press = r
		break
	}
	return nil
}

func (out *imOutput) clearScreen() {
	for i := range out.screen.Pix {
		out.screen.Pix[i] = 0
	}
}

func (out *imOutput) clearInfo() { out.info.Reset() }
func (out *imOutput) clearProf() { out.prof.Reset() }

func (out *imContext) proff(mess string, args ...interface{}) {
	_, _ = fmt.Fprintf(&out.prof, mess, args...)
}

func (out *imContext) infof(mess string, args ...interface{}) {
	_, _ = fmt.Fprintf(&out.info, mess, args...)
}

func getEnvSelector(name string) (js.Value, error) {
	selector := os.Getenv(name)
	if selector == "" {
		return js.Value{}, fmt.Errorf("no $%s given", name)
	}
	el := document.Call("querySelector", os.Getenv(name))
	if !el.Truthy() {
		return js.Value{}, fmt.Errorf("no element selected by $%s=%q", name, selector)
	}
	return el, nil
}
