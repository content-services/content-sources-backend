package adapter

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/event/message"
)

type IntrospectRequestPortIn interface {
	FromRepositoryResponse(repositoryResponse *api.RepositoryResponse) (*message.IntrospectRequestMessage, error)
	FromRepositoryBulkCreateResponse(repositoryBulkCreateResponse *api.RepositoryBulkCreateResponse) (*message.IntrospectRequestMessage, error)
}

type IntrospectRequest struct{}

func (a IntrospectRequest) FromRepositoryResponse(repositoryResponse *api.RepositoryResponse) (*message.IntrospectRequestMessage, error) {
	if repositoryResponse == nil {
		return nil, fmt.Errorf("repositoryResponse cannot be nil")
	}
	output := &message.IntrospectRequestMessage{
		Uuid: repositoryResponse.UUID,
		Url:  repositoryResponse.URL,
	}
	return output, nil
}

func (a IntrospectRequest) FromRepositoryBulkCreateResponse(repositoryBulkCreateResponse *api.RepositoryBulkCreateResponse) (*message.IntrospectRequestMessage, error) {
	if repositoryBulkCreateResponse == nil {
		return nil, fmt.Errorf("repositoryBulkCreateResponse cannot be nil")
	}
	output := &message.IntrospectRequestMessage{
		Uuid: repositoryBulkCreateResponse.Repository.UUID,
		Url:  repositoryBulkCreateResponse.Repository.URL,
	}
	return output, nil
}

func NewIntrospect() IntrospectRequest {
	return IntrospectRequest{}
}
