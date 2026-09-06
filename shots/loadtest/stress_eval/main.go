package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const password = "Stress@123456"

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type result struct {
	latency time.Duration
	kind    string // "login_ok" | "queued" | "dup_reject" | "http_err" | "net_err" | other msg
}

var sharedTransport = &http.Transport{MaxIdleConnsPerHost: 256, MaxIdleConns: 256}

var dims = `{"attendance_check":"present","expected_count":50,"actual_count":48,"teaching_documents":["syllabus","textbook"],"teaching_progress":"match","classroom_order":"good","attitude_punctual":5,"attitude_management":5,"attitude_manner":10,"content_objective":10,"content_familiar":10,"content_innovation":10,"content_ideology":10,"content_practical":10,"method_blackboard":5,"method_interactive":10,"effect_atmosphere":10,"effect_innovation":5}`

func main() {
	base := strings.TrimRight(os.Args[1], "/")
	startUID, _ := atoi(os.Args[2])
	endUID, _ := atoi(os.Args[3])
	dupN, _ := atoi(os.Args[4])
	taskID := os.Args[5]
	loginConc, submitConc := 40, 100
	if len(os.Args) > 6 {
		loginConc, _ = atoi(os.Args[6])
	}
	if len(os.Args) > 7 {
		submitConc, _ = atoi(os.Args[7])
	}
	nUsers := endUID - startUID + 1
	submitBody := fmt.Sprintf(`{"task_id":%s,"is_anonymous":false,"dimension_values":%s}`, taskID, dims)

	sem := make(chan struct{}, loginConc)
	var mu sync.Mutex
	results := make([]result, 0)
	var wg sync.WaitGroup

	// Phase 1: 并发登录
	phaseStart := time.Now()
	tokens := make([]string, endUID+1)
	for i := startUID; i <= endUID; i++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			user := fmt.Sprintf("stress_%04d", uid)
			tok, msg := doLogin(base, user)
			mu.Lock()
			defer mu.Unlock()
			if tok != "" {
				tokens[uid] = tok
				results = append(results, result{time.Since(phaseStart), "login_ok"})
			} else {
				results = append(results, result{0, "login_fail:" + msg})
			}
		}(i)
	}
	wg.Wait()
	loginElapsed := time.Since(phaseStart)
	loginOK := countKind(results, "login_ok")
	fmt.Printf("[login] %d/%d 成功, 耗时 %s, rps=%.0f\n", loginOK, nUsers, loginElapsed.Round(time.Millisecond), float64(loginOK)/loginElapsed.Seconds())
	shown := 0
	for _, r := range results {
		if strings.HasPrefix(r.kind, "login_fail") && shown < 3 {
			fmt.Printf("  [fail示例] %s\n", r.kind)
			shown++
		}
	}

	// Phase 2: 每个用户 dupN 个并发重复提交
	phaseStart = time.Now()
	sem = make(chan struct{}, submitConc)
	var totalSubmitted int64
	for uid := startUID; uid <= endUID; uid++ {
		tok := tokens[uid]
		if tok == "" {
			continue
		}
		for j := 0; j < dupN; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				lat, kind := doSubmit(base, tok, submitBody)
				atomic.AddInt64(&totalSubmitted, 1)
				mu.Lock()
				defer mu.Unlock()
				results = append(results, result{lat, kind})
			}()
		}
	}
	wg.Wait()
	submitElapsed := time.Since(phaseStart)

	// 统计
	kinds := map[string]int{}
	lat := []time.Duration{}
	for _, r := range results {
		if strings.HasPrefix(r.kind, "login_") {
			continue
		}
		kinds[r.kind]++
		if r.latency > 0 {
			lat = append(lat, r.latency)
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(p float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		return lat[int(float64(len(lat)-1)*p)]
	}
	fmt.Printf("[submit] 总请求=%d 耗时=%s rps=%.0f\n", totalSubmitted, submitElapsed.Round(time.Millisecond), float64(totalSubmitted)/submitElapsed.Seconds())
	for k, v := range kinds {
		fmt.Printf("  %-28s %d\n", k, v)
	}
	fmt.Printf("[latency] p50=%s p90=%s p99=%s max=%s\n", pct(0.5), pct(0.9), pct(0.99), lat[len(lat)-1])
}

func doLogin(base, user string) (string, string) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Transport: sharedTransport}
	body, _ := json.Marshal(map[string]string{"user_no": user, "password": password})
	resp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "net:" + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	json.Unmarshal(raw, &env)
	if env.Code != 200 {
		return "", "api:" + env.Message
	}
	for _, c := range client.Jar.Cookies(mustURL(base + "/api/v1")) {
		if c.Name == "token" {
			return c.Value, ""
		}
	}
	return "", "no_token_cookie"
}

func doSubmit(base, token, body string) (time.Duration, string) {
	client := &http.Client{Transport: sharedTransport}
	req, _ := http.NewRequest("POST", base+"/api/v1/evaluations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	t0 := time.Now()
	resp, err := client.Do(req)
	lat := time.Since(t0)
	if err != nil {
		return lat, "net_err"
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	json.Unmarshal(raw, &env)
	if env.Code == 200 && strings.Contains(string(env.Data), `"queued":true`) {
		return lat, "queued"
	}
	if strings.Contains(env.Message, "已提交过") || strings.Contains(env.Message, "请勿重复") {
		return lat, "dup_reject"
	}
	return lat, fmt.Sprintf("other[%d]:%s", env.Code, env.Message)
}

func countKind(rs []result, prefix string) int {
	n := 0
	for _, r := range rs {
		if strings.HasPrefix(r.kind, prefix) {
			n++
		}
	}
	return n
}

func mustURL(base string) *url.URL {
	u, _ := url.Parse(base)
	return u
}

func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n, nil
}
