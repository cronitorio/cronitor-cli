package lib

import (
	"math/rand"
	"time"
)

func randomMinute() int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(59)
}
