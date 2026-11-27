package main

import (
	"guanlan/internal/serviceportal"
	"log"
)

func main() {
	if err := serviceportal.Run(); err != nil {
		log.Fatal(err)
	}
}
