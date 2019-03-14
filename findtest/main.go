package main

import (
	"fmt"
	"os"
)

func main() {
	Watch(os.Args[1], func(e WatchEvent) error {
		_, err := fmt.Printf("%s\n", e)
		return err
	})
}
