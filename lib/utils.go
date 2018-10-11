package lib

import (
	"time"
	"math/rand"
)

func randomMinute() int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(59)
}
