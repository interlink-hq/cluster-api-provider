package controllers

import "time"

// requeueInterval is the default requeue interval when waiting for a resource to become ready.
const requeueInterval = 30 * time.Second

// defaultPluginPort is used when PluginPodSpec.Port is zero.
const defaultPluginPort int32 = 4000

// pluginPodLabelKey is the label key added to plugin Pods so the ClusterIP Service
// can select them.
const pluginPodLabelKey = "interlinkmachine.infrastructure.cluster.x-k8s.io/machine"
