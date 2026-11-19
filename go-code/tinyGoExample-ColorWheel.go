//go:build ignore

// Official example from tinygo.org
package main

import (
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers/ws2812"
)

func main() {
	// Set up the ring of RGB LEDs.
	pin := machine.D0
	pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	ws := ws2812.New(pin)
	leds := make([]color.RGBA, 10)

	for cycle := uint8(0); ; cycle++ {
		// Calculate a new color for each LED.
		for i := range leds {
			leds[i] = colorWheel(cycle*2 + uint8(i*8))
		}

		// Write colors to the LED ring.
		ws.WriteColors(leds)

		// Wait a bit each loop (running at around 30fps).
		time.Sleep(time.Second / 30)
	}
}

// Pick a color from the color wheel (red, then, green, then blue, and back to
// red).
func colorWheel(index uint8) color.RGBA {
	index = 255 - index
	if index < 85 {
		return color.RGBA{R: 255 - index*3, B: index * 3}
	} else if index < 170 {
		return color.RGBA{G: (index - 85) * 3, B: 255 - (index-85)*3}
	} else {
		return color.RGBA{R: (index - 170) * 3, G: 255 - (index-170)*3}
	}
}
