package cases

import "time"

type riskBand struct {
	minimum  float64
	level    string
	deadline time.Duration
}

var riskBands = []riskBand{
	{minimum: 0.5, level: "高", deadline: 2 * time.Hour},
	{minimum: 0.2, level: "中", deadline: 8 * time.Hour},
	{minimum: 0, level: "低", deadline: 24 * time.Hour},
}
