package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

func main() {
	fmt.Println("=== Memory Pressure Test ===")
	fmt.Println()

	// Print baseline.
	printMemStats("Baseline")

	// Step 1: Allocate 100MB of large buffers.
	buffers := make([][]byte, 10)
	for i := range buffers {
		buffers[i] = make([]byte, 10*1024*1024) // 10MB each
		// Fill with random data to prevent optimizations.
		for j := range buffers[i] {
			buffers[i][j] = byte(rand.Intn(256))
		}
	}
	fmt.Println("Allocated 10 x 10MB buffers (100MB total)")
	printMemStats("After allocation")

	// Step 2: Start goroutines that read/write.
	const numWorkers = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	shared := make([]int, 10000) // shared mutable state

	start := time.Now()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			local := make([]byte, 1*1024*1024) // 1MB per goroutine
			for i := 0; i < 10000; i++ {
				// Read from the large buffers.
				bufIdx := (id + i) % len(buffers)
				offset := rand.Intn(len(buffers[bufIdx]) - len(local))
				copy(local, buffers[bufIdx][offset:offset+len(local)])

				// Write to shared state.
				mu.Lock()
				shared[id*500+i%500] = int(local[0]) + i%256
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("Workers finished in %v\n", elapsed)
	printMemStats("During workers")

	// Step 3: Force GC and measure final state.
	runtime.GC()
	printMemStats("After GC")

	// Keep buffers alive to the end.
	_ = buffers
	_ = shared

	fmt.Println()
	fmt.Println("=== Summary ===")
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Printf("HeapAlloc:   %d bytes (%.2f MB)\n", ms.HeapAlloc, float64(ms.HeapAlloc)/1024/1024)
	fmt.Printf("TotalAlloc:  %d bytes (%.2f MB)\n", ms.TotalAlloc, float64(ms.TotalAlloc)/1024/1024)
	fmt.Printf("Sys:         %d bytes (%.2f MB)\n", ms.Sys, float64(ms.Sys)/1024/1024)
	fmt.Printf("NumGC:       %d\n", ms.NumGC)
	fmt.Printf("Goroutines:  %d\n", runtime.NumGoroutine())
	fmt.Println("Done.")
}

func printMemStats(label string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Printf("[%s]\n", label)
	fmt.Printf("  Alloc:      %d bytes (%.2f MB)\n", ms.Alloc, float64(ms.Alloc)/1024/1024)
	fmt.Printf("  TotalAlloc: %d bytes (%.2f MB)\n", ms.TotalAlloc, float64(ms.TotalAlloc)/1024/1024)
	fmt.Printf("  Sys:        %d bytes (%.2f MB)\n", ms.Sys, float64(ms.Sys)/1024/1024)
	fmt.Printf("  HeapAlloc:  %d bytes (%.2f MB)\n", ms.HeapAlloc, float64(ms.HeapAlloc)/1024/1024)
	fmt.Printf("  HeapSys:    %d bytes (%.2f MB)\n", ms.HeapSys, float64(ms.HeapSys)/1024/1024)
	fmt.Printf("  NumGC:      %d\n", ms.NumGC)
	fmt.Println()
}
