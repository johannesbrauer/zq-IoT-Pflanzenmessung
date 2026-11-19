//go:build ignore

package main

import (
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers/ws2812"
)

func main() {
	pin := machine.D0
	pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	ws := ws2812.NewWS2812(pin)

	LED := make([]color.RGBA, 12)
	var R, G, B uint8 = 100, 100, 200

	i := 0

	for {
		LED[i] = color.RGBA{R: R, G: G, B: B}
		ws.WriteColors(LED)

		// Advance to the next LED and wrap around at 12
		i++
		if i >= len(LED) {
			i = 0
		}

		// Increment colors (uint8 automatically wraps from 255 back to 0)
		R += 50
		G -= 50
		B += 150

		// Pause briefly so the animation is visible and the LEDs can latch
		time.Sleep(50 * time.Millisecond)
	}

}
