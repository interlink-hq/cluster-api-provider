package controllers

import "time"

// requeueInterval is the default requeue interval when waiting for a resource to become ready.
const requeueInterval = 30 * time.Second
