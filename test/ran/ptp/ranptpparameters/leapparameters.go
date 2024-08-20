package ranptpparameters

const (
	LeapConfigMap = "leap-configmap"
	DateFormat    = "2 Jan 2006"
)

// PatchInterfaceValue ia patch struct for updating the leap-config map.
type PatchInterfaceValue struct {
	Op    string            `json:"op"`
	Path  string            `json:"path"`
	Value map[string]string `json:"value"`
}
