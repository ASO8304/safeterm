package main

import (
	"fmt"
	"safeterm/internal/config"
)

func main() {
	fmt.Println("SafeTerm Agent - M0 Foundation")
	fmt.Println("Status:", config.Status())
}
