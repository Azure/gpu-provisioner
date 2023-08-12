package instance

import "time"

// Instance a struct to isolate weather vm or vmss
type Instance struct {
	LaunchTime   time.Time
	State        *string
	ID           *string
	ImageID      *string
	Type         *string
	CapacityType *string
	SubnetID     *string
	Tags         map[string]string
}
