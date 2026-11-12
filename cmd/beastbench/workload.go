package main

import (
	"fmt"
	"strings"
)

// OpType is a benchmarked operation kind.
type OpType int

const (
	OpRead OpType = iota
	OpUpdate
	OpInsert
	OpScan
	OpRMW
	numOps
)

var opNames = [numOps]string{"read", "update", "insert", "scan", "rmw"}

func (o OpType) String() string { return opNames[o] }

// Workload is a YCSB-style operation mix; proportions sum to 1.
type Workload struct {
	Name    string
	Desc    string
	Mix     [numOps]float64
	Latest  bool // read-latest key distribution (D)
	Uniform bool // workload forces a distribution regardless of flag (none today)
}

var workloads = map[string]Workload{
	"A": {Name: "A", Desc: "50% read / 50% update", Mix: mix(OpRead, .5, OpUpdate, .5)},
	"B": {Name: "B", Desc: "95% read / 5% update", Mix: mix(OpRead, .95, OpUpdate, .05)},
	"C": {Name: "C", Desc: "100% read", Mix: mix(OpRead, 1.0)},
	"D": {Name: "D", Desc: "95% read-latest / 5% insert", Mix: mix(OpRead, .95, OpInsert, .05), Latest: true},
	"E": {Name: "E", Desc: "95% short scan / 5% insert", Mix: mix(OpScan, .95, OpInsert, .05)},
	"F": {Name: "F", Desc: "50% read / 50% read-modify-write (conditional put)", Mix: mix(OpRead, .5, OpRMW, .5)},
}

func mix(kv ...any) [numOps]float64 {
	var m [numOps]float64
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(OpType)] = kv[i+1].(float64)
	}
	return m
}

func lookupWorkload(name string) (Workload, error) {
	w, ok := workloads[strings.ToUpper(name)]
	if !ok {
		return w, fmt.Errorf("unknown workload %q (want A-F)", name)
	}
	return w, nil
}

// pick maps u in [0,1) to an operation.
func (w Workload) pick(u float64) OpType {
	var acc float64
	last := OpRead
	for op := OpType(0); op < numOps; op++ {
		if w.Mix[op] == 0 {
			continue
		}
		last = op
		acc += w.Mix[op]
		if u < acc {
			return op
		}
	}
	return last
}
