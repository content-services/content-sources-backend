package jfrog_bridge

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
)

// PartnerHandler delivers remediated artifacts to one partner. Each handler
// fetches what it needs from the Lightwell registry, runs its own integrity
// and content-embargo checks, uploads, and verifies the delivery.
type PartnerHandler interface {
	Name() string
	// Process runs the delivery pipeline for one remediation.
	Process(ctx context.Context, rem Remediation) error
	// Status returns the partner's non-secret configuration for the admin API.
	Status() map[string]interface{}
	// Health probes the partner's dependencies. The body is returned as JSON.
	Health(ctx context.Context) (healthy bool, body interface{})
}

// partner describes a partner known to the bridge. Each partner consumes the
// bridge topic in its own consumer group, so offsets are committed independently.
type partner struct {
	name            string
	displayName     string
	enabled         func() bool
	consumerGroupID func() string
	adminFeature    func() config.Feature
	newHandler      func() (PartnerHandler, error)
}

// partners is the registry of partners, keyed by partner.name.
var partners = []partner{
	jfrogPartner,
	sonatypePartner,
	sonarSourcePartner,
}

func lookupPartner(name string) (partner, bool) {
	for _, p := range partners {
		if p.name == name {
			return p, true
		}
	}
	return partner{}, false
}
