package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"strconv"
	"path/filepath"
)

type Session struct {
	// List of urls to hit in the test. Example:
	// GET https://google.com
	// POST https://foobar.com
	Targets string `json:"targets"`

	// Duration of the test. Example: 10s, 30s, 1m, 5m
	Duration string `json:"duration"`

	// Number of requests per second per node
	Rate string `json:"rate"`

	// Execute by steps until reach the rate
	UseSteps bool `json:"use_steps"`

	// Array of duration steps
	DurationSteps []string `json:"duration_steps"`

	// Array of rate steps
	RateSteps []string `json:"rate_steps"`
}

var (
	// State represents current node state
	state = "pending"

	// Path where to save vegerate benchmark data. We cant use any other directory
	// because heroku's filesystem is not writable. We can still write to /tmp!
	reportPath = "/tmp/vegeta"

	// Vegeta binary path
	vegetaPath = "./bin/vegeta"
)

func runSession(session Session) {
	state = "working"
	log.Println("starting session")

	defer func() {
		state = "done"
		log.Println("session is finished")
	}()

	// Remove an existing report files if it exists
	files, err := filepath.Glob(reportPath + "*")
	if err != nil {
	    panic(err)
	}

	for _, f := range files {
	    if err := os.Remove(f); err != nil {
	        panic(err)
	    }
	}

	if session.UseSteps == true {
		for i := 0; i < len(session.DurationSteps); i++ {
			log.Println("Run step: ", i + 1)
			duration := session.DurationSteps[i]
			rate := session.RateSteps[i]
			runCommand(rate, duration, session.Targets, i)
		}
	} else {
		log.Println("Run once")
		runCommand(session.Rate, session.Duration, session.Targets, 0)
	}
}

func runCommand(rate string, duration string, targets string, reportNumber int) {
	opts := []string{
		"attack",
		"-timeout=10s",
		"-rate=" + rate,
		"-duration=" + duration,
		"-output", reportPath + strconv.Itoa(reportNumber),
	}

	// Setup vegeta runner
	cmd := exec.Command(vegetaPath, opts...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(targets)

	if err := cmd.Start(); err != nil {
		log.Println("unable to start vegeta command:", err)
		return
	}

	if err := cmd.Wait(); err != nil {
		log.Println("command exited:", err)
	}
}

func RunSession(w http.ResponseWriter, r *http.Request) {
	if state == "working" {
		http.Error(w, "another session is in progress", 400)
		return
	}

	session := Session{}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&session); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	go runSession(session)
}

func GetReport(w http.ResponseWriter, r *http.Request) {
	reportNumber, err := strconv.Atoi(r.URL.Query().Get("report_number"))
	fmt.Println("REPORT NUMBER: " + r.URL.Query().Get("report_number"))
    if err != nil {
        reportNumber = 0
    }

	f, err := os.Open(reportPath + strconv.Itoa(reportNumber))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer f.Close()

	w.Header().Add("Content-Type", "application/octet-stream")
	reader := bufio.NewReader(f)
	reader.WriteTo(w)
}

func GetState(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, state)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	http.HandleFunc("/state", GetState)
	http.HandleFunc("/report", GetReport)
	http.HandleFunc("/run", RunSession)

	log.Println("starting server on port", port)
	http.ListenAndServe(":"+port, nil)
}
