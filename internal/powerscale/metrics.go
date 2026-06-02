package powerscale

// Label is a single metric label name-value pair.
type Label struct {
	Name  string
	Value string
}

// Sample is one exported metric data point.
type Sample struct {
	Name   string
	Labels []Label
	Value  float64
}
