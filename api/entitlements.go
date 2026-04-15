package main

type planLimits struct {
	MaxStorageBytes int64
	MaxProjects     int   // 0 = unlimited
	MaxFileSize     int64 // 0 = no limit
	DeployHistory   bool
	PrivateSites    bool
	Rollback        bool
}

var plans = map[string]planLimits{
	"tinkerer": {
		MaxStorageBytes: 10 * 1024 * 1024 * 1024,
		MaxProjects:     1000,
		MaxFileSize:     250 * 1024 * 1024,
		DeployHistory:   false,
		PrivateSites:    false,
		Rollback:        false,
	},
	"pro": {
		MaxStorageBytes: 50 * 1024 * 1024 * 1024,
		MaxProjects:     0,
		MaxFileSize:     0,
		DeployHistory:   true,
		PrivateSites:    true,
		Rollback:        true,
	},
	"team": {
		MaxStorageBytes: 50 * 1024 * 1024 * 1024, // per seat
		MaxProjects:     0,
		MaxFileSize:     0,
		DeployHistory:   true,
		PrivateSites:    true,
		Rollback:        true,
	},
}

func getPlanLimits(plan string) planLimits {
	if limits, ok := plans[plan]; ok {
		return limits
	}
	return plans["tinkerer"]
}
