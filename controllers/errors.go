package controllers

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
)

// isNotFound returns true when err, or any error within an aggregate tree, is
// a Kubernetes "not found" error.  This is necessary because the CAPI patch
// helper wraps errors in a kerrors.Aggregate which does not implement Unwrap,
// preventing the standard apierrors.IsNotFound from working through the chain.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	// Unwrap pkg/errors chains (withStack, withMessage).
	type causer interface{ Cause() error }
	if c, ok := err.(causer); ok {
		return isNotFound(c.Cause())
	}
	// Unwrap kerrors.Aggregate.
	if agg, ok := err.(kerrors.Aggregate); ok {
		for _, e := range agg.Errors() {
			if isNotFound(e) {
				return true
			}
		}
	}
	return false
}
