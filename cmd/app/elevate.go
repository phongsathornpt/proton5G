package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
)

// maybeElevate re-execs this process under sudo when not root.
// Needed for sysfs autosuspend, serial /dev/ttyUSB*, and USBDEVFS_RESET.
// Skip with -no-elevate or FM350_NO_ELEVATE=1.
func maybeElevate(noElevate bool) {
	if noElevate || os.Getenv("FM350_NO_ELEVATE") == "1" {
		return
	}
	if os.Geteuid() == 0 {
		return
	}
	// Already running the elevated child (belt-and-suspenders).
	if os.Getenv("FM350_ELEVATED") == "1" {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Printf("[WARN] cannot resolve executable for sudo re-exec: %v — run with: sudo go run ./cmd/app", err)
		return
	}

	args := append([]string{exe}, os.Args[1:]...)
	cmd := exec.Command("sudo", args...)
	cmd.Env = append(os.Environ(), "FM350_ELEVATED=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("[INFO] Not root — re-running with sudo for serial/sysfs access (pass -no-elevate to skip)")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
		fmt.Fprintf(os.Stderr, "sudo elevate failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "hint: sudo go run ./cmd/app   or   sudo ./fm350-manager\n")
		os.Exit(1)
	}
	os.Exit(0)
}
