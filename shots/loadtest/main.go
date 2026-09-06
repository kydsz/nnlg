package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := os.Args[1]
	total, _ := atoi(os.Args[2])
	conc, _ := atoi(os.Args[3])
	body := ""
	if len(os.Args) > 4 {
		body = os.Args[4]
	}

	var ok, fail, non200 int64
	lat := make([]time.Duration, 0, total)
	var mu sync.Mutex
	start := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, conc)
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: conc + 10}}

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t0 := time.Now()
			var resp *http.Response
			var err error
			if body == "" {
				resp, err = client.Get(url)
			} else {
				resp, err = client.Post(url, "application/json", bytes.NewReader([]byte(body)))
			}
			d := time.Since(t0)
			if err != nil {
				atomic.AddInt64(&fail, 1)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				atomic.AddInt64(&non200, 1)
			}
			atomic.AddInt64(&ok, 1)
			mu.Lock()
			lat = append(lat, d)
			mu.Unlock()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(p float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		return lat[int(float64(len(lat)-1)*p)]
	}
	fmt.Printf("total=%d ok=%d fail=%d non200=%d elapsed=%s rps=%.0f\np50=%s p90=%s p99=%s max=%s\n",
		total, ok, fail, non200, elapsed.Round(time.Millisecond), float64(ok)/elapsed.Seconds(),
		pct(0.5), pct(0.9), pct(0.99), lat[len(lat)-1])
}

func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n, nil
}
