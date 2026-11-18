package jfrog_bridge

import (
	"context"
	"errors"

	"github.com/content-services/content-sources-backend/pkg/config"
)

const (
	sonarSourcePartnerName     = "sonarsource"
	sonarSourceConsumerGroupID = "content-sources-sonarsource-bridge"
)

var errSonarSourceContractUndefined = errors.New("sonarsource: upload contract is not defined")

// sonarSourcePartner stays disabled until the SonarSource upload contract
// (API, authentication, payload, path layout, ecosystems) is agreed.
var sonarSourcePartner = partner{
	name:            sonarSourcePartnerName,
	displayName:     "SonarSource",
	enabled:         func() bool { return false },
	consumerGroupID: func() string { return sonarSourceConsumerGroupID },
	adminFeature:    func() config.Feature { return config.Feature{} },
	newHandler:      func() (PartnerHandler, error) { return sonarSourceHandler{}, nil },
}

type sonarSourceHandler struct{}

func (sonarSourceHandler) Name() string { return sonarSourcePartnerName }

// Process never reports success until the upload contract exists.
func (sonarSourceHandler) Process(context.Context, Remediation) error {
	return errSonarSourceContractUndefined
}

func (sonarSourceHandler) Status() map[string]interface{} {
	return map[string]interface{}{"enabled": false}
}

func (sonarSourceHandler) Health(context.Context) (bool, interface{}) {
	return false, map[string]interface{}{
		"healthy": false,
		"error":   errSonarSourceContractUndefined.Error(),
	}
}
