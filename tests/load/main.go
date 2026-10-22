// Command load measures the in-process baseline recorded in
// docs/adr/0001-gate-distributed-workers.md and re-checks the gate in
// docs/measurements. It is a measurement tool, not runtime infrastructure:
// nothing in the server imports it and CI does not run it. Point it at a
// throwaway database only, because it creates projects and tasks.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	base := flag.String("base", "http://127.0.0.1:8080", "boAIrd base URL")
	tasks := flag.Int("tasks", 120, "tasks per claim scenario")
	claimConcurrency := flag.String("claim-concurrency", "1,8,32", "comma-separated claim concurrency levels")
	readConcurrency := flag.String("read-concurrency", "1,8,32", "comma-separated read concurrency levels")
	readsPerWorker := flag.Int("reads-per-worker", 20, "read operations per worker")
	flag.Parse()

	token := os.Getenv("BOAIRD_LOCAL_AUTH_TOKEN")
	if len(token) != 64 {
		fmt.Fprintln(os.Stderr, "BOAIRD_LOCAL_AUTH_TOKEN must be the 64-hex-character administrator token")
		os.Exit(2)
	}
	claimLevels := parseLevels(*claimConcurrency)
	readLevels := parseLevels(*readConcurrency)

	runner := &runner{
		base:   *base,
		token:  token,
		client: &http.Client{Transport: &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second}},
	}

	projectID, err := runner.createProject()
	if err != nil {
		fail("create project", err)
	}
	fmt.Printf("project %s\n", projectID)

	latencies, elapsed, err := runner.createTasks(projectID, *tasks)
	if err != nil {
		fail("create tasks", err)
	}
	report("create task sequential", latencies, elapsed)

	for _, concurrency := range claimLevels {
		ready, err := runner.createReadyTasks(projectID, *tasks)
		if err != nil {
			fail("prepare ready tasks", err)
		}
		latencies, elapsed, err := runner.claim(ready, concurrency)
		if err != nil {
			fail("claim tasks", err)
		}
		report(fmt.Sprintf("claim READY->WORKING concurrency=%d", concurrency), latencies, elapsed)
	}

	ids, err := runner.taskIDs(projectID)
	if err != nil {
		fail("list tasks", err)
	}
	for _, concurrency := range readLevels {
		latencies, elapsed := runner.getTasks(ids, concurrency, *readsPerWorker)
		report(fmt.Sprintf("get task concurrency=%d", concurrency), latencies, elapsed)
	}
	for _, concurrency := range readLevels {
		latencies, elapsed := runner.listTasks(projectID, concurrency, *readsPerWorker)
		report(fmt.Sprintf("list tasks limit=50 concurrency=%d", concurrency), latencies, elapsed)
	}
}

type runner struct {
	base   string
	token  string
	client *http.Client
}

func (r *runner) do(method, path string, body, out any) (resultErr error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, r.base+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+r.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close response body: %w", err))
		}
	}()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out != nil {
		return json.Unmarshal(payload, out)
	}
	return nil
}

func (r *runner) createProject() (string, error) {
	var project struct {
		ID string `json:"id"`
	}
	err := r.do(http.MethodPost, "/api/v1/projects", map[string]string{"name": "load-baseline", "description": "ADR 0001 measurement"}, &project)
	return project.ID, err
}

func (r *runner) createTasks(projectID string, count int) ([]float64, time.Duration, error) {
	latencies := make([]float64, 0, count)
	started := time.Now()
	for index := 0; index < count; index++ {
		operationStarted := time.Now()
		err := r.do(http.MethodPost, "/api/v1/projects/"+projectID+"/tasks", taskInput(fmt.Sprintf("baseline-%d", index)), nil)
		latencies = append(latencies, time.Since(operationStarted).Seconds())
		if err != nil {
			return nil, 0, err
		}
	}
	return latencies, time.Since(started), nil
}

type taskPayload struct {
	ID string `json:"id"`
}

func (r *runner) createReadyTasks(projectID string, count int) ([]string, error) {
	ids := make([]string, 0, count)
	for index := 0; index < count; index++ {
		var created taskPayload
		if err := r.do(http.MethodPost, "/api/v1/projects/"+projectID+"/tasks", taskInput(fmt.Sprintf("claim-%d", index)), &created); err != nil {
			return nil, err
		}
		if err := r.do(http.MethodPost, "/api/v1/tasks/"+created.ID+"/transitions", map[string]string{"status": "READY", "reason": "baseline"}, nil); err != nil {
			return nil, err
		}
		ids = append(ids, created.ID)
	}
	return ids, nil
}

func taskInput(title string) map[string]string {
	return map[string]string{"title": title, "description": "measurement", "priority": "NORMAL", "reason": "baseline"}
}

func (r *runner) taskIDs(projectID string) ([]string, error) {
	var page struct {
		Items []taskPayload `json:"items"`
	}
	if err := r.do(http.MethodGet, "/api/v1/projects/"+projectID+"/tasks?limit=100", nil, &page); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("project %s has no tasks", projectID)
	}
	return ids, nil
}

func (r *runner) claim(ids []string, concurrency int) ([]float64, time.Duration, error) {
	work := make(chan string)
	latencies := make([][]float64, concurrency)
	failures := make(chan error, concurrency)
	started := time.Now()
	var group sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for id := range work {
				operationStarted := time.Now()
				err := r.do(http.MethodPost, "/api/v1/tasks/"+id+"/transitions", map[string]string{"status": "WORKING", "reason": "claim"}, nil)
				latencies[worker] = append(latencies[worker], time.Since(operationStarted).Seconds())
				if err != nil {
					failures <- err
					return
				}
			}
		}(worker)
	}
	for _, id := range ids {
		work <- id
	}
	close(work)
	group.Wait()
	close(failures)
	elapsed := time.Since(started)
	if err := <-failures; err != nil {
		return nil, 0, err
	}
	return flatten(latencies), elapsed, nil
}

func (r *runner) getTasks(ids []string, concurrency, perWorker int) ([]float64, time.Duration) {
	latencies := make([][]float64, concurrency)
	started := time.Now()
	var group sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for operation := 0; operation < perWorker; operation++ {
				operationStarted := time.Now()
				err := r.do(http.MethodGet, "/api/v1/tasks/"+ids[(worker*perWorker+operation)%len(ids)], nil, &taskPayload{})
				latencies[worker] = append(latencies[worker], time.Since(operationStarted).Seconds())
				if err != nil {
					fmt.Fprintln(os.Stderr, "get task:", err)
					return
				}
			}
		}(worker)
	}
	group.Wait()
	return flatten(latencies), time.Since(started)
}

func (r *runner) listTasks(projectID string, concurrency, perWorker int) ([]float64, time.Duration) {
	latencies := make([][]float64, concurrency)
	path := "/api/v1/projects/" + projectID + "/tasks?limit=50"
	started := time.Now()
	var group sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for operation := 0; operation < perWorker; operation++ {
				operationStarted := time.Now()
				var page map[string]any
				err := r.do(http.MethodGet, path, nil, &page)
				latencies[worker] = append(latencies[worker], time.Since(operationStarted).Seconds())
				if err != nil {
					fmt.Fprintln(os.Stderr, "list tasks:", err)
					return
				}
			}
		}(worker)
	}
	group.Wait()
	return flatten(latencies), time.Since(started)
}

func parseLevels(raw string) []int {
	levels := []int{}
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 1 {
			fmt.Fprintf(os.Stderr, "invalid concurrency level %q\n", part)
			os.Exit(2)
		}
		levels = append(levels, value)
	}
	return levels
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}

func flatten(groups [][]float64) []float64 {
	values := []float64{}
	for _, group := range groups {
		values = append(values, group...)
	}
	return values
}

func report(name string, latencies []float64, elapsed time.Duration) {
	if len(latencies) == 0 || elapsed <= 0 {
		fmt.Printf("%-42s no samples\n", name)
		return
	}
	sort.Float64s(latencies)
	fmt.Printf("%-42s n=%-5d %8.0f ops/s  p50=%8.2fms p95=%8.2fms p99=%8.2fms max=%8.2fms\n",
		name, len(latencies), float64(len(latencies))/elapsed.Seconds(),
		percentile(latencies, 0.50)*1000, percentile(latencies, 0.95)*1000, percentile(latencies, 0.99)*1000, percentile(latencies, 1)*1000)
}

func percentile(sorted []float64, fraction float64) float64 {
	index := int(float64(len(sorted)-1) * fraction)
	return sorted[index]
}
