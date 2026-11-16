package service

import (
	"sync"
	"testing"
	"time"
)

func TestBenchmarkRunnerSharesOneAsynchronousGate(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	runner := NewBenchmarkRunner(func() {
		once.Do(func() { close(started) })
		<-release
	})

	if !runner.Start() {
		t.Fatal("first Start was refused")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("benchmark did not start asynchronously")
	}
	if runner.Start() {
		t.Fatal("second Start was accepted while the first was running")
	}
	if !runner.Running() {
		t.Fatal("runner did not report the in-flight benchmark")
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for runner.Running() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runner.Running() {
		t.Fatal("benchmark gate remained claimed after completion")
	}
}

func TestBenchmarkRunnerWaitWaitsForTheInFlightRun(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := NewBenchmarkRunner(func() {
		close(started)
		<-release
	})

	if !runner.Start() {
		t.Fatal("Start was refused")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("benchmark did not start")
	}
	if runner.Wait(10 * time.Millisecond) {
		t.Fatal("Wait reported a free gate while the run was in flight")
	}
	waited := make(chan bool, 1)
	go func() { waited <- runner.Wait(time.Second) }()
	time.Sleep(10 * time.Millisecond)
	close(release)
	if !<-waited {
		t.Fatal("Wait did not observe the run finishing")
	}
	if runner.Running() {
		t.Fatal("runner still reports the gate claimed")
	}
}

func TestBenchmarkRunnerRunIsSynchronousAndSharesTheGate(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	runner := NewBenchmarkRunner(func() {
		close(started)
		<-release
	})

	done := make(chan bool, 1)
	go func() { done <- runner.Run() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Run did not invoke the benchmark")
	}
	if runner.Start() {
		t.Fatal("Start was accepted while Run held the gate")
	}
	close(release)
	if !<-done {
		t.Fatal("Run reported failure")
	}
	if runner.Running() {
		t.Fatal("Run left the gate claimed")
	}
}
