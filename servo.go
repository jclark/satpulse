package main

import (
	"fmt"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/tai"
)

type Servo struct {
	clk *phc.Clock
}

func (s *Servo) Sample(master tai.Time, slave tai.Time) {
	off := master.Sub(slave)
	fmt.Printf("Servo: GPS  %v, PHC %v, master offset: %v\n", master, slave, off)
}
