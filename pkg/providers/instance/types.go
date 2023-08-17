package instance

// Instance a struct to isolate weather vm or vmss
type Instance struct {
	Name         *string // agentPoolName or instance/vmName
	State        *string
	ID           *string
	ImageID      *string
	Type         *string
	CapacityType *string
	SubnetID     *string
	Tags         map[string]*string
	Labels       map[string]string
}
