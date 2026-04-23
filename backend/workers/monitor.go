package workers

import "fmt"

func StartMonitor() {
	go func() {
		fmt.Println("Worker started...")
	}()
}
