//go:build ignore

package main

import (
	"machine"
	"time"
)

func main() {
	// Pin D2 des XIAO ESP32-C3 als Ausgang konfigurieren
	pin := machine.D0
	pin.Configure(machine.PinConfig{Mode: machine.PinOutput})

	for {
		pin.High() // LED an
		time.Sleep(1 * time.Second)

		pin.Low() // LED aus
		time.Sleep(1 * time.Second)
	}
}
