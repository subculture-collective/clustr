package affinity

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed data/historical_defaults_v1.txt
var historicalDefaultsV1 string

const HistoricalDefaultVersion = "historical-defaults-v1"
const HistoricalDefaultRepresentativePrior = .5

func HistoricalDefaults() (map[string]bool,string) {
	values:=map[string]bool{}
	for _,line:=range strings.Split(historicalDefaultsV1,"\n"){if name:=strings.ToLower(strings.TrimSpace(line));name!=""{values[name]=true}}
	return values,fmt.Sprintf("%x",sha256.Sum256([]byte(historicalDefaultsV1)))
}

// RepresentativeScore applies the default prior only after clustering. The
// membership algorithm never calls this function.
func RepresentativeScore(name string,withinAffinity,exclusivity,confidence,genericness float64)float64{
	score:=withinAffinity*.4+exclusivity*.3+confidence*.2+(1-genericness)*.1
	defaults,_:=HistoricalDefaults();if defaults[strings.ToLower(strings.TrimSpace(name))]{score*=HistoricalDefaultRepresentativePrior};return score
}
