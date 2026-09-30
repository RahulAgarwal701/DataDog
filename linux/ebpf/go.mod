module github.com/minidd/ebpf-agent

go 1.25.0

require (
	github.com/cilium/ebpf v0.22.0
	github.com/minidd/topology v0.0.0
	golang.org/x/sys v0.43.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace github.com/minidd/topology => ../topology
