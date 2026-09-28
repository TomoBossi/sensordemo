package main

import "time"

// clock is the time of day as demos see it: the wall clock, except when
// frames are rendered offline on simulated time (the README's GIFs).
var clock = time.Now
