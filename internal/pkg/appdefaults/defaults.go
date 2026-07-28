package appdefaults

import "time"

// HTTP / process defaults for cmd/app and shared intervals.
const (
	HTTPPort           = 8080
	HTTPBind           = "0.0.0.0"
	HistoryCap         = 180
	WatchInterval      = 5 * time.Second
	HistorySave        = 60 * time.Second
	SSEInterval        = 2 * time.Second
	StatusPollInterval = 2 * time.Second // background AT/USB status poller
)

// Recovery policy defaults used by usecase.
const (
	ATFailResetStreak  = 3
	ATResetCooldown    = 60 * time.Second
	ATRediscoverEvery  = 30 * time.Second // min gap between AT port re-probes
)

// DefaultCID is the primary PDP context ID.
const DefaultCID = 1
