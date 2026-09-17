package model

import "time"

// EnvironmentUtilization is the physical utilization summary for one environment.
type EnvironmentUtilization struct {
	EnvironmentID         string
	EnvironmentName       string
	TrainingCenterID      string
	EnvironmentTypeCode   string
	PeriodFrom            time.Time
	PeriodTo              time.Time
	AvailableHours        float64
	ReservedHours         float64
	MaintenanceHours      float64
	UtilizationPercentage float64
}
