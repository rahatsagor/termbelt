package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SpeedData struct {
	Provider           string   `json:"provider"`
	DownloadMbps       float64  `json:"download_mbps"`
	UploadMbps         *float64 `json:"upload_mbps,omitempty"`
	LatencyMS          float64  `json:"http_latency_ms"`
	DownloadedBytes    int64    `json:"downloaded_bytes"`
	UploadedBytes      int64    `json:"uploaded_bytes"`
	DownloadSeconds    float64  `json:"download_seconds"`
	UploadSeconds      float64  `json:"upload_seconds,omitempty"`
	Servers            []string `json:"servers"`
	DownloadStopReason string   `json:"download_stop_reason"`
	UploadStopReason   string   `json:"upload_stop_reason,omitempty"`
	Partial            bool     `json:"partial"`
}

var fastScriptPattern = regexp.MustCompile(`<script[^>]+src=["']([^"']*app[^"']*\.js)["']`)
var fastTokenPattern = regexp.MustCompile(`token\s*:\s*["']([A-Za-z0-9_-]{12,})["']`)

func (e *Engine) fastTargets(ctx context.Context) ([]string, error) {
	b, status, err := e.get(ctx, "https://fast.com")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("Fast.com returned HTTP %d", status)
	}
	script := fastScriptPattern.FindSubmatch(b)
	if len(script) < 2 {
		return nil, fmt.Errorf("Fast.com changed its script layout; try --provider cloudflare")
	}
	base, _ := url.Parse("https://fast.com")
	path, err := url.Parse(string(script[1]))
	if err != nil {
		return nil, err
	}
	scriptURL := base.ResolveReference(path)
	if scriptURL.Scheme != "https" || scriptURL.Host != "fast.com" {
		return nil, fmt.Errorf("unexpected Fast.com script URL")
	}
	js, status, err := e.get(ctx, scriptURL.String())
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("Fast.com script returned HTTP %d", status)
	}
	match := fastTokenPattern.FindSubmatch(js)
	if len(match) < 2 {
		return nil, fmt.Errorf("Fast.com changed its public API token format; try --provider cloudflare")
	}
	address := "https://api.fast.com/netflix/speedtest/v2?https=true&urlCount=3&token=" + url.QueryEscape(string(match[1]))
	body, status, err := e.get(ctx, address)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("Fast.com API returned HTTP %d; try --provider cloudflare", status)
	}
	var data struct {
		Targets []struct {
			URL string `json:"url"`
		} `json:"targets"`
	}
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	targets := []string{}
	for _, target := range data.Targets {
		u, err := url.Parse(target.URL)
		if err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && strings.HasSuffix(u.Path, "/speedtest") {
			targets = append(targets, u.String())
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("Fast.com returned no download servers")
	}
	return targets, nil
}
func reserveBytes(counter *atomic.Int64, chunk, max int64) int64 {
	for {
		current := counter.Load()
		if current >= max {
			return 0
		}
		size := min(chunk, max-current)
		if counter.CompareAndSwap(current, current+size) {
			return size
		}
	}
}

type byteCounter struct{ total *atomic.Int64 }

func (c byteCounter) Write(p []byte) (int, error) { c.total.Add(int64(len(p))); return len(p), nil }

type transferStats struct {
	bytes      int64
	seconds    float64
	stopReason string
	failures   []string
}

func (e *Engine) transfer(ctx context.Context, targets []string, upload bool, seconds time.Duration, maxBytes int64, emit Emit) (int64, float64, error) {
	stats, err := e.transferDetailed(ctx, targets, upload, seconds, maxBytes, emit)
	return stats.bytes, stats.seconds, err
}

func downloadTarget(target string, size int64) string {
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	if strings.EqualFold(u.Hostname(), "speed.cloudflare.com") {
		q := u.Query()
		q.Set("bytes", fmt.Sprint(size))
		u.RawQuery = q.Encode()
	} else if strings.HasSuffix(u.Path, "/speedtest") {
		u.Path += fmt.Sprintf("/range/0-%d", size-1)
	}
	return u.String()
}

func (e *Engine) transferDetailed(ctx context.Context, targets []string, upload bool, seconds time.Duration, maxBytes int64, emit Emit) (transferStats, error) {
	if len(targets) == 0 || seconds <= 0 || maxBytes <= 0 {
		return transferStats{}, fmt.Errorf("speed measurement needs a server, positive duration and byte budget")
	}
	if err := ctx.Err(); err != nil {
		return transferStats{}, err
	}
	const maxChunk = int64(8 << 20)
	workers := int(min(int64(3), maxBytes))
	initialChunk := min(maxChunk, maxBytes/int64(workers))
	if upload {
		initialChunk = min(initialChunk, 256<<10)
	}
	// Reserve each connection's first request before starting workers so even a
	// small data budget measures concurrent throughput.
	var transferred, reserved atomic.Int64
	first := make([]int64, workers)
	payloads := make([][]byte, workers)
	for i := range first {
		first[i] = reserveBytes(&reserved, initialChunk, maxBytes)
		if upload {
			payloads[i] = make([]byte, initialChunk)
			if _, err := rand.Read(payloads[i]); err != nil {
				return transferStats{}, err
			}
		}
	}
	phaseCtx, cancel := context.WithTimeout(ctx, seconds)
	defer cancel()
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	start := time.Now()
	client := *e.Client
	client.Timeout = 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			chunk := initialChunk
			payload := payloads[index]
			size := first[index]
			for {
				if phaseCtx.Err() != nil {
					return
				}
				if size == 0 {
					return
				}
				target := targets[index%len(targets)]
				method := http.MethodGet
				var reader io.Reader
				if upload {
					method = http.MethodPost
					if int64(len(payload)) < size {
						payload = make([]byte, size)
						if _, err := rand.Read(payload); err != nil {
							errors <- err
							return
						}
					}
					reader = bytes.NewReader(payload[:size])
				} else {
					target = downloadTarget(target, size)
				}
				req, err := http.NewRequestWithContext(phaseCtx, method, target, reader)
				if err != nil {
					errors <- err
					return
				}
				req.Header.Set("User-Agent", "termbelt/1.0")
				req.Header.Set("Accept-Encoding", "identity")
				req.Header.Set("Cache-Control", "no-cache")
				if upload {
					req.Header.Set("Content-Type", "application/octet-stream")
					req.ContentLength = size
				}
				requestStart := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					if phaseCtx.Err() == nil {
						errors <- err
					}
					return
				}
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					resp.Body.Close()
					errors <- fmt.Errorf("speed server returned HTTP %d", resp.StatusCode)
					return
				}
				if !upload {
					mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
					encoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
					if mediaType == "text/html" || mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") || (encoding != "" && encoding != "identity") || resp.Uncompressed {
						resp.Body.Close()
						errors <- fmt.Errorf("speed server returned an error page or compressed payload")
						return
					}
				}
				if upload {
					_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
					if err == nil {
						transferred.Add(size)
					}
				} else {
					var n int64
					n, err = io.Copy(byteCounter{&transferred}, io.LimitReader(resp.Body, size))
					reserved.Add(-(size - n))
					if n == 0 && err == nil {
						err = fmt.Errorf("speed server returned an empty download")
					}
				}
				resp.Body.Close()
				if err != nil {
					if phaseCtx.Err() == nil {
						errors <- err
					}
					return
				}
				if upload {
					// Larger reusable requests avoid a fixed 256 KiB/RTT ceiling on
					// fast links. Aim for half a second, with bounded growth.
					elapsed := max(time.Since(requestStart).Seconds(), 0.001)
					chunk = min(maxChunk, max(256<<10, min(chunk*4, int64(float64(size)*0.5/elapsed))))
				} else {
					chunk = maxChunk
				}
				size = reserveBytes(&reserved, chunk, maxBytes)
			}
		}(i)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	label := "DOWNLOAD"
	if upload {
		label = "UPLOAD"
	}
	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(start).Seconds()
			mbps := float64(transferred.Load()) * 8 / elapsed / 1e6
			report(emit, Progress{Message: fmt.Sprintf("Measuring %s · %.1f MiB transferred", strings.ToLower(label), float64(transferred.Load())/(1<<20)), Fraction: min(elapsed/seconds.Seconds(), 1), Metrics: []Metric{{Label: label, Value: fmt.Sprintf("%.1f", mbps), Unit: "Mbps"}}})
		case <-done:
			elapsed := time.Since(start).Seconds()
			stats := transferStats{bytes: transferred.Load(), seconds: elapsed, stopReason: "byte-limit"}
			close(errors)
			for err := range errors {
				stats.failures = append(stats.failures, err.Error())
			}
			if phaseCtx.Err() != nil {
				stats.stopReason = "duration"
			} else if len(stats.failures) > 0 {
				stats.stopReason = "server-error"
			}
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			if transferred.Load() == 0 {
				if len(stats.failures) > 0 {
					return stats, fmt.Errorf("speed measurement failed: %s", strings.Join(stats.failures, "; "))
				}
				return stats, fmt.Errorf("no data transferred before the measurement deadline; retry with --duration 20")
			}
			return stats, nil
		}
	}
}
func (e *Engine) measureLatency(ctx context.Context, target string) (float64, error) {
	samples := []float64{}
	client := *e.Client
	client.Timeout = 5 * time.Second
	for i := 0; i < 3; i++ {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
		if err != nil {
			return 0, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return 0, fmt.Errorf("latency endpoint returned HTTP %d", resp.StatusCode)
		}
		if readErr != nil {
			return 0, readErr
		}
		samples = append(samples, float64(time.Since(start).Microseconds())/1000)
	}
	return (samples[0] + samples[1] + samples[2]) / 3, nil
}
func (e *Engine) Speed(ctx context.Context, r Request, emit Emit) (Result, error) {
	provider := strings.ToLower(r.Opt("provider", "fast"))
	if provider != "fast" && provider != "cloudflare" {
		return Result{}, fmt.Errorf("provider must be fast or cloudflare")
	}
	seconds, err := r.Int("duration", 8, 2, 30)
	if err != nil {
		return Result{}, err
	}
	maxMB, err := r.Int("max-mb", 128, 1, 1024)
	if err != nil {
		return Result{}, err
	}
	report(emit, Progress{Message: "Finding speed-test servers", Fraction: 0})
	targets := []string{"https://speed.cloudflare.com/__down?bytes=8388608"}
	latencyTarget := "https://speed.cloudflare.com/__down?bytes=0"
	if provider == "fast" {
		targets, err = e.fastTargets(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("Fast.com: %w", err)
		}
		u, _ := url.Parse(targets[0])
		u.Path += "/range/0-0"
		latencyTarget = u.String()
	}
	report(emit, Progress{Message: "Measuring unloaded HTTP latency", Fraction: 0})
	latency, err := e.measureLatency(ctx, latencyTarget)
	if err != nil {
		return Result{}, err
	}
	download, err := e.transferDetailed(ctx, targets, false, time.Duration(seconds)*time.Second, int64(maxMB)<<20, emit)
	if err != nil {
		return Result{}, err
	}
	downloaded, downloadSeconds := download.bytes, download.seconds
	data := SpeedData{Provider: provider, DownloadMbps: float64(downloaded) * 8 / downloadSeconds / 1e6, LatencyMS: latency, DownloadedBytes: downloaded, DownloadSeconds: downloadSeconds, Servers: []string{}, DownloadStopReason: download.stopReason, Partial: len(download.failures) > 0}
	for _, target := range targets {
		u, _ := url.Parse(target)
		data.Servers = append(data.Servers, u.Hostname())
	}
	metrics := []Metric{{Label: "DOWNLOAD", Value: fmt.Sprintf("%.1f", data.DownloadMbps), Unit: "Mbps"}, {Label: "HTTP LATENCY", Value: fmt.Sprintf("%.1f", latency), Unit: "ms"}}
	notes := []string{fmt.Sprintf("Transfers stop after %d seconds or %d MiB per direction, whichever comes first. Throughput is an aggregate average across up to three HTTPS connections.", seconds, maxMB), "HTTP latency includes server response time and is measured before the transfer; this test does not measure packet loss or loaded latency."}
	for _, failure := range download.failures {
		notes = append(notes, "Partial download measurement: "+failure)
	}
	if provider == "cloudflare" && !r.Bool("download-only") {
		upload, uploadErr := e.transferDetailed(ctx, []string{"https://speed.cloudflare.com/__up"}, true, time.Duration(seconds)*time.Second, int64(maxMB)<<20, emit)
		if uploadErr != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			notes = append(notes, "Upload test failed: "+uploadErr.Error())
			data.Partial = true
			data.UploadStopReason = "server-error"
		} else {
			uploaded, uploadSeconds := upload.bytes, upload.seconds
			mbps := float64(uploaded) * 8 / uploadSeconds / 1e6
			data.UploadMbps = &mbps
			data.UploadedBytes = uploaded
			data.UploadSeconds = uploadSeconds
			data.UploadStopReason = upload.stopReason
			data.Partial = data.Partial || len(upload.failures) > 0
			for _, failure := range upload.failures {
				notes = append(notes, "Partial upload measurement: "+failure)
			}
			metrics = append(metrics, Metric{Label: "UPLOAD", Value: fmt.Sprintf("%.1f", mbps), Unit: "Mbps"})
		}
	} else if provider == "fast" {
		notes = append(notes, "Fast.com mode measures Netflix download throughput. Use --provider cloudflare for both download and upload.")
	}
	if downloadSeconds < 2 {
		notes = append(notes, "The download reached its data limit in under two seconds; increase --max-mb for a more stable measurement.")
	}
	return Result{Title: "Internet speed", Summary: map[string]string{"fast": "Fast.com · Netflix CDN", "cloudflare": "Cloudflare edge network"}[provider], Metrics: metrics, Sections: []Section{{Title: "MEASUREMENT", Rows: []Row{row("Servers", strings.Join(data.Servers, ", ")), row("Downloaded", fmt.Sprintf("%.2f MiB", float64(downloaded)/(1<<20))), row("Download elapsed", fmt.Sprintf("%.2f s", downloadSeconds)), row("Uploaded", fmt.Sprintf("%.2f MiB", float64(data.UploadedBytes)/(1<<20)))}}}, Notes: notes, Data: data}, nil
}
