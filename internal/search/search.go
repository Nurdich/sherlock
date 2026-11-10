// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Package search checks a username against Sherlock site definitions.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Nurdich/sherlock/internal/notify"
	"github.com/Nurdich/sherlock/internal/result"
	"github.com/Nurdich/sherlock/internal/sites"
)

// UserAgent is the browser identity used for site probes.
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:129.0) Gecko/20100101 Firefox/129.0"

// Hit is the stored outcome for one site.
type Hit struct {
	Name         string
	URLMain      string
	URLUser      string
	Result       result.Result
	HTTPStatus   string
	ResponseBody string
}

// Config controls one username search.
type Config struct {
	Timeout    time.Duration
	Proxy      string
	Dump       bool
	DumpWriter io.Writer
}

// Run probes every site for username and notifies as the Python CLI does:
// illegal usernames are reported while probes are started, then HTTP results
// are reported in manifest order.
func Run(ctx context.Context, username string, list []sites.Site, n notify.Notifier, cfg Config) ([]Hit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if n == nil {
		n = noop{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.DumpWriter == nil {
		cfg.DumpWriter = os.Stdout
	}
	n.Start(username)

	hits := make([]Hit, len(list))
	type probeJob struct {
		index   int
		site    sites.Site
		profile string
		probe   string
	}
	var jobs []probeJob
	for i, site := range list {
		profile := ProfileURL(site.URL, username)
		hits[i] = Hit{Name: site.Name, URLMain: site.URLMain, URLUser: profile}
		if !site.UsernameAllowed(username) {
			res := result.Result{
				Username:    username,
				SiteName:    site.Name,
				SiteURLUser: profile,
				Status:      result.Illegal,
			}
			hits[i].URLUser = ""
			hits[i].HTTPStatus = ""
			hits[i].Result = res
			n.Update(res)
			continue
		}
		jobs = append(jobs, probeJob{index: i, site: site, profile: profile, probe: ProbeURL(site.URLProbe, profile, username)})
	}
	if len(jobs) == 0 {
		return hits, nil
	}

	client, err := newClient(cfg.Timeout, cfg.Proxy)
	if err != nil {
		return nil, err
	}
	workers := len(jobs)
	if workers > 20 {
		workers = 20
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	outcomes := make([]Observation, len(list))
	for _, job := range jobs {
		wg.Add(1)
		go func(job probeJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			obs, err := probe(ctx, client, job.site, username, job.probe)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			outcomes[job.index] = obs
		}(job)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return nil, err
	default:
	}

	for _, job := range jobs {
		obs := outcomes[job.index]
		status, contextText := Decide(job.site, obs)
		httpStatus := "?"
		if obs.Err == nil {
			httpStatus = fmt.Sprintf("%d", obs.StatusCode)
		}
		res := result.Result{
			Username:    username,
			SiteName:    job.site.Name,
			SiteURLUser: job.profile,
			Status:      status,
			QueryTime:   obs.Elapsed,
			Context:     contextText,
		}
		hits[job.index].URLUser = job.profile
		hits[job.index].HTTPStatus = httpStatus
		hits[job.index].ResponseBody = obs.Body
		hits[job.index].Result = res
		if cfg.Dump {
			writeDump(cfg.DumpWriter, job.site, username, job.profile, status, obs)
		}
		n.Update(res)
	}
	return hits, nil
}

func probe(ctx context.Context, client *http.Client, site sites.Site, username, probeURL string) (Observation, error) {
	method := site.RequestMethod
	if method == "" {
		if site.ErrorTypeExact == "status_code" {
			method = http.MethodHead
		} else {
			method = http.MethodGet
		}
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut:
	default:
		return Observation{}, fmt.Errorf("Unsupported request_method for %s", probeURL)
	}

	var body io.Reader
	if site.RequestPayload != nil {
		encoded, err := json.Marshal(Interpolate(site.RequestPayload, username))
		if err != nil {
			elapsed := 0.0
			return Observation{Err: err, Elapsed: &elapsed}, nil
		}
		body = bytes.NewReader(encoded)
	}
	rawURL, err := requestURL(probeURL)
	if err != nil {
		return Observation{Err: err}, nil
	}
	allowRedirect := site.ErrorTypeExact != "response_url"
	reqCtx := context.WithValue(ctx, redirectKey{}, allowRedirect)
	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, body)
	if err != nil {
		return Observation{Err: err}, nil
	}
	req.Header.Set("User-Agent", UserAgent)
	for key, value := range site.Headers {
		if strings.EqualFold(key, "Host") {
			req.Host = value
			continue
		}
		req.Header.Set(key, value)
	}
	if site.RequestPayload != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Seconds()
	if err != nil {
		return Observation{Err: err, Elapsed: &elapsed}, nil
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(resp.Body)
	obs := Observation{
		StatusCode: resp.StatusCode,
		Body:       string(payload),
		Elapsed:    &elapsed,
	}
	if readErr != nil {
		obs.Err = readErr
	}
	return obs, nil
}

func writeDump(out io.Writer, site sites.Site, username, profile string, status result.Status, obs Observation) {
	fmt.Fprintln(out, "+++++++++++++++++++++")
	fmt.Fprintf(out, "TARGET NAME   : %s\n", site.Name)
	fmt.Fprintf(out, "USERNAME      : %s\n", username)
	fmt.Fprintf(out, "TARGET URL    : %s\n", profile)
	fmt.Fprintf(out, "TEST METHOD   : %s\n", pyList(site.ErrorTypes))
	if site.ErrorCodePresent {
		if site.ErrorCodeIsList {
			fmt.Fprintf(out, "STATUS CODES  : %s\n", pyInts(site.ErrorCodes))
		} else if len(site.ErrorCodes) > 0 {
			fmt.Fprintf(out, "STATUS CODES  : %d\n", site.ErrorCodes[0])
		}
	}
	fmt.Fprintln(out, "Results...")
	if obs.Err == nil {
		fmt.Fprintf(out, "RESPONSE CODE : %d\n", obs.StatusCode)
	}
	if site.ErrorMsgPresent {
		if site.ErrorMsgIsList {
			fmt.Fprintf(out, "ERROR TEXT    : %s\n", pyList(site.ErrorMsgs))
		} else if len(site.ErrorMsgs) > 0 {
			fmt.Fprintf(out, "ERROR TEXT    : %s\n", site.ErrorMsgs[0])
		}
	}
	fmt.Fprintln(out, ">>>>> BEGIN RESPONSE TEXT")
	if obs.Err == nil {
		fmt.Fprintln(out, obs.Body)
	}
	fmt.Fprintln(out, "<<<<< END RESPONSE TEXT")
	fmt.Fprintf(out, "VERDICT       : %s\n", status.String())
	fmt.Fprintln(out, "+++++++++++++++++++++")
}

type noop struct{}

func (noop) Start(string)         {}
func (noop) Update(result.Result) {}
func (noop) Finish()              {}
