package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
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
	pollInterval := flag.Duration("poll", appdefaults.StatusPollInterval, "Background status (AT/USB) poll interval")
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

	atClient := repository.NewTieredClient(targetPort)
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

	hotspotRuntime := appdefaults.HotspotRuntimeDir
	if rd := strings.TrimSpace(os.Getenv("RUNTIME_DIRECTORY")); rd != "" {
		hotspotRuntime = rd
	}
	hotspotConfigPath := appdefaults.HotspotConfigFile
	if sd := strings.TrimSpace(os.Getenv("STATE_DIRECTORY")); sd != "" {
		hotspotConfigPath = filepath.Join(sd, "hotspot.json")
	}

	inventory := usecase.NewCachedInventory(usecase.InventoryFuncs{
		ListModemsFn:      repository.ListModems,
		ListMBIMFn:        repository.ListMBIMDevices,
		MBIMAvailableFn:   repository.Available,
		MBIMInstallHintFn: repository.InstallHint,
	}, 15*time.Second)
	// Prime before ModemService exists so sysfs walks / tty probes never happen
	// while the service state mutex is held on the normal UI inventory path.
	inventory.Prime(domain.DefaultFM350.Vendor, domain.DefaultFM350.Product, atClient.PortName())

	svc := usecase.NewModemService(usecase.ModemServiceConfig{
		USB:               usb,
		AT:                atClient,
		History:           hist,
		MBIM:              repository.NewMBIM(),
		Net:               repository.NewNetRepo(),
		Hotspot:           repository.NewHotspotRepo(hotspotRuntime),
		HotspotConfigFile: hotspotConfigPath,
		Discover:          usecase.DiscoverFunc(repository.DiscoverATPort),
		Inventory:         inventory,
		Vendor:            domain.DefaultFM350.Vendor,
		Product:           domain.DefaultFM350.Product,
	})
	if err := svc.LoadHotspotConfigFile(); err != nil {
		log.Printf("[WARN] Load hotspot config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var bg sync.WaitGroup
	startBG := func(name string, fn func()) {
		bg.Add(1)
		go func() {
			defer bg.Done()
			fn()
			log.Printf("[INFO] background %s stopped", name)
		}()
	}

	startBG("watchdog", func() { svc.RunWatchdog(ctx, *watchInterval) })
	startBG("status-poller", func() {
		log.Printf("[INFO] Status poller interval %s", *pollInterval)
		svc.RunStatusPoller(ctx, *pollInterval)
	})

	if *historyFile != "" {
		path := *historyFile
		every := *historySaveEvery
		startBG("history-save", func() {
			ticker := time.NewTicker(every)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := hist.SaveFile(path); err != nil {
						log.Printf("[WARN] Save history: %v", err)
					}
				}
			}
		})
	}

	httpHandler := handler.NewServer(svc, token)
	startBG("sse-hub", func() { httpHandler.Run(ctx) })

	addr := fmt.Sprintf("%s:%d", *bind, *port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      httpHandler.Routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE long-lived
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

	// Tear down WiFi hotspot (hostapd/dnsmasq/NAT) before exiting.
	if _, err := svc.HotspotStop(); err != nil {
		log.Printf("[WARN] Hotspot stop: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)

	// Wait for background workers (poller, watchdog, history) up to remaining budget.
	done := make(chan struct{})
	go func() {
		bg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Println("[WARN] background workers did not stop within 5s")
	}

	if *historyFile != "" {
		if err := hist.SaveFile(*historyFile); err != nil {
			log.Printf("[WARN] Final history save: %v", err)
		}
	}
	_ = atClient.Close()
	log.Println("[INFO] Goodbye.")
}
