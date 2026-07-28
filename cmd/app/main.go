package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fm350-monitor/internal/handler"
	"fm350-monitor/internal/pkg/appdefaults"
	"fm350-monitor/internal/pkg/domain"
	"fm350-monitor/internal/repository"
	"fm350-monitor/internal/usecase"
)

func main() {
	port := flag.Int("port", appdefaults.HTTPPort, "WebUI HTTP listen port")
	bind := flag.String("bind", appdefaults.HTTPBind, "HTTP bind address (default localhost for safety)")
	serialPort := flag.String("serial", "", "Override AT serial port (e.g. /dev/ttyUSB0)")
	watchInterval := flag.Duration("watch", appdefaults.WatchInterval, "USB watchdog poll interval")
	historySize := flag.Int("history", appdefaults.HistoryCap, "In-memory signal history capacity")
	historyFile := flag.String("history-file", "", "Optional JSON file to load/save signal history")
	historySaveEvery := flag.Duration("history-save", appdefaults.HistorySave, "How often to persist history when -history-file is set")
	noElevate := flag.Bool("no-elevate", false, "Do not re-exec with sudo when not root")
	apiToken := flag.String("token", "", "Optional shared API token (or env FM350_API_TOKEN); protects WebUI + API")
	flag.Parse()

	// Re-exec under sudo when needed (serial, sysfs, USBDEVFS_RESET).
	maybeElevate(*noElevate)

	token := strings.TrimSpace(*apiToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("FM350_API_TOKEN"))
	}
	if token != "" {
		log.Printf("[INFO] API token auth enabled (Authorization Bearer / X-API-Token / ?token=)")
	}

	log.Println("Starting FM350-GL USB Modem Manager & Watchdog...")

	usb := repository.NewWatchdog(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
	if err := usb.DisableAutosuspend(); err != nil {
		log.Printf("[WARN] Failed to set global USB autosuspend: %v", err)
	}

	targetPort := *serialPort
	if targetPort == "" {
		discovered, err := repository.DiscoverATPort(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product)
		if err != nil {
			log.Printf("[WARN] AT port discovery: %v", err)
		}
		if discovered != "" {
			targetPort = discovered
			log.Printf("[INFO] Discovered AT command port: %s", targetPort)
		} else {
			targetPort = repository.DefaultATPort
			log.Printf("[INFO] Could not auto-discover AT port, defaulting to: %s", targetPort)
		}
	} else {
		log.Printf("[INFO] Using override AT serial port: %s", targetPort)
	}

	atClient := repository.NewClient(targetPort)
	if err := atClient.Connect(); err != nil {
		log.Printf("[WARN] Failed to open AT serial port (%s): %v. Will retry on status polls.", targetPort, err)
	} else {
		log.Printf("[INFO] Connected to AT serial port (%s)", targetPort)
	}

	hist := repository.NewRing(*historySize)
	if *historyFile != "" {
		if err := hist.LoadFile(*historyFile); err != nil {
			log.Printf("[WARN] Load history file: %v", err)
		} else {
			log.Printf("[INFO] Loaded signal history from %s (%d samples)", *historyFile, hist.Len())
		}
	}

	svc := usecase.NewModemService(usecase.ModemServiceConfig{
		USB:      usb,
		AT:       atClient,
		History:  hist,
		MBIM:     repository.NewMBIM(),
		Net:      repository.NewNetRepo(),
		Discover: usecase.DiscoverFunc(repository.DiscoverATPort),
		Inventory: usecase.InventoryFuncs{
			ListModemsFn:      repository.ListModems,
			ListMBIMFn:        repository.ListMBIMDevices,
			MBIMAvailableFn:   repository.Available,
			MBIMInstallHintFn: repository.InstallHint,
		},
		Vendor:  domain.DefaultFM350.Vendor,
		Product: domain.DefaultFM350.Product,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.RunWatchdog(ctx, *watchInterval)

	if *historyFile != "" {
		go func() {
			ticker := time.NewTicker(*historySaveEvery)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := hist.SaveFile(*historyFile); err != nil {
						log.Printf("[WARN] Save history: %v", err)
					}
				}
			}
		}()
	}

	go func() { svc.Status() }()

	httpHandler := handler.NewServer(svc, token)
	addr := fmt.Sprintf("%s:%d", *bind, *port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      httpHandler.Routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
	}

	go func() {
		log.Printf("[INFO] WebUI active at http://%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("[INFO] Shutting down FM350-GL Manager...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	if *historyFile != "" {
		if err := hist.SaveFile(*historyFile); err != nil {
			log.Printf("[WARN] Final history save: %v", err)
		}
	}
	_ = atClient.Close()
	log.Println("[INFO] Goodbye.")
}
